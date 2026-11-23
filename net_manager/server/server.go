package server

import (
	"NetManager/env"
	"NetManager/handlers"
	"NetManager/logger"
	"NetManager/model"
	"NetManager/mqtt"
	"NetManager/network"
	"NetManager/p2p"
	"NetManager/proxy"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

const IP_UPDATE_TIMER = 2 * time.Minute

type registerRequest struct {
	ClientID          string `json:"client_id"`
	ClusterAddress    string `json:"cluster_address"`
	NodePublicAddress string `json:"node_public_address"`
	// P2P mode fields. Mode "" or "cluster" => cluster mode.
	Mode        string   `json:"mode"`
	GossipKey   string   `json:"gossip_key"` // gossip PSK (derived from token/enrollment)
	Seeds       []string `json:"seeds"`      // static bootstrap peers
	GossipPort  int      `json:"gossip_port"`
	ControlPort int      `json:"control_port"` // NodeEngine control API port (default 50105)
}

type DeployResponse struct {
	ServiceName string `json:"serviceName"`
	NsAddress   string `json:"nsAddress"`
}

func update() {
	for {
		time.Sleep(IP_UPDATE_TIMER)
		defaultLink := network.GetOutboundIP()
		if model.NetConfig.NodePublicAddress != defaultLink.String() {
			logger.InfoLogger().Printf("Updating NodePublicAddress from %s to %s", model.NetConfig.NodePublicAddress, defaultLink.String())
			// this timer can fire before /register has created Env
			if Env != nil {
				Env.Resolver().SetLocalNode(defaultLink)
			}
			// update service in the cluster; p2p mode has no MQTT, and re-advertising
			// the new host address into the gossip registry is driven by the NodeEngine
			//for each service instance in the worker, update the public address
			if Env != nil && !model.P2PMode {
				for _, si := range Env.GetTableEntriesOnNode() {
					err := mqtt.NotifyAddressChange(si.Appname, si.Instancenumber, defaultLink.String(), model.NetConfig.NodePublicPort)
					if err != nil {
						logger.ErrorLogger().Println("[ERROR]:", err)
					}
				}
			}
			model.NetConfig.NodePublicAddress = defaultLink.String()
		}
	}
}

func createListener(port int) net.Listener {
	if port <= 0 {
		logger.InfoLogger().Println("Starting NetManager on unix socket /etc/netmanager/netmanager.sock")
		_ = os.Remove("/etc/netmanager/netmanager.sock")
		listener, err := net.Listen("unix", "/etc/netmanager/netmanager.sock")
		if err != nil {
			log.Fatalf("Could not create listener: %s", err)
		}
		return listener
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("Could not create listener: %s", err)
	}
	return listener
}

func HandleRequests(port int) {
	// registration bootstrap server
	initRouter := mux.NewRouter().StrictSlash(true)
	initRouter.HandleFunc("/register", register).Methods("POST")

	initServer = &http.Server{Handler: initRouter}
	initListener := createListener(port)

	if err := initServer.Serve(initListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Registration server failed: %s", err)
	}

	// main server with all registered routes
	mainRouter := mux.NewRouter().StrictSlash(true)
	mainRouter.HandleFunc("/register", register).Methods("POST")
	registerP2PRoutes(mainRouter)
	handlers.RegisterAllManagers(&model.WorkerID, model.NetConfig.NodePublicAddress, model.NetConfig.NodePublicPort, mainRouter)

	mainListener := createListener(port)
	log.Fatal(http.Serve(mainListener, mainRouter))
}

var (
	Env        *env.Environment
	Proxy      *proxy.Tunnel
	initServer *http.Server
	// P2P mode state; nil in cluster mode.
	P2PMembership *p2p.Membership
	P2PRegistry   *p2p.Registry
	P2PResolver   p2p.ServiceResolver
)

/*
Endpoint: /register
Usage: used to initialize the Network manager. The network manager must know his local subnetwork.
Method: POST
Request Json:

	{
		client_id:string # id of the worker node
	}

Response: 200 or Failure code
*/
func register(writer http.ResponseWriter, request *http.Request) {
	logger.InfoLogger().Println("Received registration request, registering the NetManager to the Cluster")

	reqBody, _ := io.ReadAll(request.Body)
	var requestStruct registerRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Println(requestStruct)

	// drop the request if the node is already initialized
	if model.WorkerID != "" {
		if model.WorkerID == requestStruct.ClientID {
			logger.InfoLogger().Printf("Node already initialized")
			writer.WriteHeader(http.StatusOK)
		} else {
			logger.InfoLogger().Printf("Attempting to re-initialize a node with a different worker ID")
			writer.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	// 0.0.0.0 is the default when netcfg.json is created
	if model.NetConfig.NodePublicAddress == "" || model.NetConfig.NodePublicAddress == "0.0.0.0" {
		if requestStruct.NodePublicAddress != "" {
			// when NodeEngine had an explicit public IP configured, use it
			model.NetConfig.NodePublicAddress = requestStruct.NodePublicAddress
		} else {
			// when NodeEngine had no IP configured, get the default outbound IP
			defaultLink := network.GetOutboundIP()
			model.NetConfig.NodePublicAddress = defaultLink.String()
			go update()
		}
	}

	model.WorkerID = requestStruct.ClientID

	//Use default cluster address given by NodeEngine version >= v0.4.302
	if requestStruct.ClusterAddress != "" {
		model.NetConfig.ClusterUrl = requestStruct.ClusterAddress
	}

	//log registration startup
	logger.InfoLogger().Printf(
		"STARTUP_CONFIG: Node=%s:%s | Mode=%s | Cluster=%s:%s",
		model.NetConfig.NodePublicAddress,
		model.NetConfig.NodePublicPort,
		requestStruct.Mode,
		model.NetConfig.ClusterUrl,
		model.NetConfig.ClusterMqttPort,
	)

	if requestStruct.Mode == "p2p" {
		registerP2P(requestStruct)
	} else {
		registerCluster(requestStruct)
	}

	logger.InfoLogger().Printf("NetManager is now running 🟢")
	writer.WriteHeader(http.StatusOK)

	// Shut down stage 1 server so HandleRequests proceeds to stage 2
	if initServer != nil {
		go func() {
			_ = initServer.Shutdown(context.Background())
		}()
	}
}

// registerCluster wires the NetManager for cluster mode (MQTT control plane).
func registerCluster(req registerRequest) {
	// initialize mqtt connection to the broker
	mqtt.InitNetMqttClient(req.ClientID, model.NetConfig.ClusterUrl, model.NetConfig.ClusterMqttPort, model.NetConfig.MqttCert, model.NetConfig.MqttKey)

	Proxy = newProxy()
	// initialize the Env Manager on the fixed shared subnet
	Env = env.NewEnvironmentFixedSubnet(Proxy.HostTUNDeviceName, "", "")
	startDatapath()
}

// registerP2P wires the NetManager for peer-to-peer mode: gossip membership + registry
// resolver instead of the cluster MQTT table query.
func registerP2P(req registerRequest) {
	// Mark p2p mode so the MQTT-dependent code paths (deployment status, interest,
	// address-change notifications) are skipped — there is no MQTT client in p2p mode.
	model.P2PMode = true

	controlPort := req.ControlPort
	if controlPort == 0 {
		controlPort = 50105
	}
	registry := p2p.NewRegistry()
	membership, err := p2p.StartMembership(p2p.MembershipConfig{
		NodeUUID:   req.ClientID,
		BindAddr:   "",
		GossipPort: req.GossipPort,
		SecretKey:  p2p.DeriveGossipKey(req.GossipKey),
		Seeds:      req.Seeds,
		// Membership events are forwarded to the paired NodeEngine, which owns
		// failover rescheduling and PENDING retry.
		OnNodeLeave: func(nodeUUID string, lost []p2p.RegistryEntry) {
			notifyNodeEngine(controlPort, "/ctrl/nodeleave", map[string]interface{}{
				"node_uuid": nodeUUID,
				"lost":      lost,
			})
		},
		OnNodeJoin: func(nodeUUID string) {
			notifyNodeEngine(controlPort, "/ctrl/nodejoin", map[string]interface{}{
				"node_uuid": nodeUUID,
			})
		},
	}, registry)
	if err != nil {
		logger.ErrorLogger().Printf("p2p: failed to start membership: %v", err)
	}
	P2PMembership = membership
	P2PRegistry = registry
	P2PResolver = p2p.NewResolver(registry, req.ClientID, nil)

	// mDNS discovery: advertise our gossip endpoint on the LAN and, whenever
	// this node finds itself alone (stale seeds after DHCP churn / reboots), browse for
	// peers and re-join. Trust is unaffected — the PSK still gates admission.
	gossipPort := req.GossipPort
	if gossipPort == 0 {
		gossipPort = 50104
	}
	go runMDNSDiscovery(req.ClientID, gossipPort, membership)

	// data-plane encryption: the AEAD key is derived from the same network
	// secret as the gossip PSK
	Proxy = newProxy()
	if req.GossipKey != "" {
		if err := Proxy.EnableDataEncryption(proxy.DeriveDataKey(req.GossipKey)); err != nil {
			logger.ErrorLogger().Printf("p2p: data-plane encryption setup failed: %v", err)
		}
	}

	// fixed shared subnet, resolution via the p2p registry
	Env = env.NewEnvironmentFixedSubnet(Proxy.HostTUNDeviceName, "", "")
	Env.Resolver().UseBackend(P2PResolver)
	startDatapath()

	go p2pRouteRefreshLoop()
}

// newProxy creates the proxy tunnel bound to this host's local IP.
func newProxy() *proxy.Tunnel {
	ipstring, _ := network.GetLocalIPandIface()
	localIP, err := netip.ParseAddr(ipstring)
	if err != nil {
		log.Fatalf("Unable to parse the local IP %q: %s", ipstring, err)
	}
	return proxy.New(localIP)
}

// startDatapath connects the proxy to Env's resolver and starts it. Shared by
// both modes, once Env and Proxy exist.
func startDatapath() {
	// namespace IPs are only unique per node: lookups must prefer this node's instances
	Env.Resolver().SetLocalNode(net.ParseIP(model.NetConfig.NodePublicAddress))
	// must be set before Listen starts the read loops, or Datapath sees a nil resolver
	Proxy.SetResolver(Env.Resolver())
	Proxy.Listen()

	handlers.SetEnvironmentForAllManagers(Env)
}

// P2PRouteRefreshInterval is how often the translation table is reconciled against the
// gossip registry. The cluster control plane pushes these updates over MQTT; p2p has no
// push, so this is what lets a scaled-out replica join the round-robin rotation and a
// departed one drop out of it. The registry is a local in-memory map, so the poll is
// cheap and the interval is set by how quickly a new replica should start taking traffic.
var P2PRouteRefreshInterval = 5 * time.Second

func p2pRouteRefreshLoop() {
	for {
		time.Sleep(P2PRouteRefreshInterval)
		Env.Resolver().RefreshRoutes(P2PResolver)
	}
}

// runMDNSDiscovery advertises this node's gossip endpoint via mDNS and periodically
// browses for peers while the node is alone — zero-config stale-seed recovery on a LAN.
// First-contact trust stays with the token/enrollment; the gossip PSK
// still decides who we actually talk to.
func runMDNSDiscovery(nodeUUID string, gossipPort int, membership *p2p.Membership) {
	if membership == nil {
		return
	}
	disc := p2p.NewMDNSDiscovery()
	if err := disc.Advertise(nodeUUID, gossipPort); err != nil {
		logger.ErrorLogger().Printf("p2p: mDNS advertise failed (continuing without): %v", err)
	}
	for {
		if membership.NumMembers() <= 1 {
			peers := disc.Browse(3 * time.Second)
			self := fmt.Sprintf(":%d", gossipPort)
			candidates := make([]string, 0, len(peers))
			for _, p := range peers {
				// skip obvious self-references (same port on a local address is joined
				// harmlessly by memberlist anyway, but avoid the noise)
				if strings.HasSuffix(p, self) && strings.HasPrefix(p, "127.") {
					continue
				}
				candidates = append(candidates, p)
			}
			if len(candidates) > 0 {
				membership.JoinLater(candidates)
			}
		}
		time.Sleep(30 * time.Second)
	}
}

// notifyNodeEngine posts a membership event to the local NodeEngine control API.
// Best-effort: the NodeEngine may be restarting; failover convergence is retried by design.
// The control API uses roster-pinned mTLS; the NetManager authenticates with
// the SAME host identity (/etc/oakestra/p2p/node.crt) — it runs on the same node.
func notifyNodeEngine(controlPort int, path string, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client, scheme := nodeEngineClient()
	resp, err := client.Post(
		fmt.Sprintf("%s://127.0.0.1:%d%s", scheme, controlPort, path),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		logger.DebugLogger().Printf("p2p: NodeEngine notify %s failed: %v", path, err)
		return
	}
	_ = resp.Body.Close()
}

// nodeEngineClient builds an HTTPS client using the shared host identity when the p2p
// credentials exist, falling back to plain HTTP otherwise (pre-genesis).
func nodeEngineClient() (*http.Client, string) {
	cert, err := tls.LoadX509KeyPair("/etc/oakestra/p2p/node.crt", "/etc/oakestra/p2p/node.key")
	if err != nil {
		return &http.Client{Timeout: 5 * time.Second}, "http"
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			// Local loopback to our own paired NodeEngine: the server presents the very
			// same self-signed host cert, so chain validation is meaningless here.
			InsecureSkipVerify: true, //nolint:gosec
		}},
	}, "https"
}
