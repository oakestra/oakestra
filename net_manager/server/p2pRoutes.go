package server

import (
	"NetManager/logger"
	"NetManager/p2p"
	"NetManager/proxy"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// p2pRoutes exposes the gossip mesh to the paired NodeEngine over the local control
// connection (unix socket / localhost). The NodeEngine uses these to place services,
// advertise deployed services, and render `services ls`.
// All handlers 404 until the NetManager is registered in p2p mode.
func registerP2PRoutes(router *mux.Router) {
	router.HandleFunc("/p2p/members", p2pMembersHandler).Methods("GET")
	router.HandleFunc("/p2p/services", p2pServicesHandler).Methods("GET")
	router.HandleFunc("/p2p/service", p2pAdvertiseHandler).Methods("POST")
	router.HandleFunc("/p2p/service/{job}/{instance}", p2pWithdrawHandler).Methods("DELETE")
	router.HandleFunc("/p2p/rotate", p2pRotateHandler).Methods("POST")
}

// POST /p2p/rotate {gossip_key} — rotate the gossip PSK and the derived data-plane key
// at runtime (revocation). Called by the paired NodeEngine after a revoke.
func p2pRotateHandler(writer http.ResponseWriter, request *http.Request) {
	if !p2pReady(writer) {
		return
	}
	var req struct {
		GossipKey string `json:"gossip_key"`
	}
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil || req.GossipKey == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := P2PMembership.RotateKey(p2p.DeriveGossipKey(req.GossipKey)); err != nil {
		logger.ErrorLogger().Printf("p2p: gossip key rotation failed: %v", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := Proxy.EnableDataEncryption(proxy.DeriveDataKey(req.GossipKey)); err != nil {
		logger.ErrorLogger().Printf("p2p: data key rotation failed: %v", err)
	}
	writer.WriteHeader(http.StatusOK)
}

func p2pReady(writer http.ResponseWriter) bool {
	if P2PMembership == nil || P2PRegistry == nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return false
	}
	return true
}

// GET /p2p/members -> [{uuid, addr}]
func p2pMembersHandler(writer http.ResponseWriter, request *http.Request) {
	if !p2pReady(writer) {
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(P2PMembership.Members())
}

// GET /p2p/services -> full registry dump ([]RegistryEntry)
func p2pServicesHandler(writer http.ResponseWriter, request *http.Request) {
	if !p2pReady(writer) {
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(P2PRegistry.Snapshot())
}

// POST /p2p/service — advertise a locally-hosted service into the gossip registry.
// The NodeEngine sends the entry with ServiceIPs it allocated; the NetManager fills in
// the namespace IPs (it owns them) and the node's public endpoint.
func p2pAdvertiseHandler(writer http.ResponseWriter, request *http.Request) {
	if !p2pReady(writer) {
		return
	}
	body, _ := io.ReadAll(request.Body)
	var entry p2p.RegistryEntry
	if err := json.Unmarshal(body, &entry); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	for i := range entry.Instances {
		inst := &entry.Instances[i]
		if inst.NamespaceIP == "" {
			if v4, v6, ok := Env.GetDeployedServiceIPs(entry.JobName, inst.InstanceNumber); ok {
				inst.NamespaceIP = v4.String()
				if v6 != nil {
					inst.NamespaceIPv6 = v6.String()
				}
			}
		}
	}
	if P2PResolver != nil {
		P2PResolver.Advertise(entry)
	} else {
		P2PRegistry.Put(entry)
	}

	// Load this node's own service into the LOCAL translation table.
	//
	// The proxy translates a packet's SOURCE namespace IP into that instance's ServiceIP
	// (Datapath.convertToInstanceIp), and that lookup is deliberately local-only, because
	// "the networking component MUST have all the entries for the local deployed
	// services". In cluster mode the deploy path satisfies that via the MQTT table query
	// (handlers.updateInternalProxyDataStructures -> RefreshServiceTable). In p2p mode
	// that path is skipped and resolution only ever fills in *destinations*, so without
	// this no container could originate overlay traffic at all. The registry already
	// holds the entry advertised above, so reloading the job picks it up together with
	// any replicas on other nodes.
	if P2PResolver != nil {
		Env.Resolver().ReloadJob(P2PResolver, entry.JobName)
	}

	logger.InfoLogger().Printf("p2p: NodeEngine advertised %s (%d instance(s))", entry.JobName, len(entry.Instances))
	writer.WriteHeader(http.StatusOK)
}

// DELETE /p2p/service/{job}/{instance} — withdraw a local instance from the registry.
func p2pWithdrawHandler(writer http.ResponseWriter, request *http.Request) {
	if !p2pReady(writer) {
		return
	}
	vars := mux.Vars(request)
	instance, err := strconv.Atoi(vars["instance"])
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	if P2PResolver != nil {
		P2PResolver.Withdraw(vars["job"], instance)
	}
	// Drop the local rows too. The periodic refresh would eventually notice, but until it
	// did, the proxy could still pick an instance that has just been undeployed.
	Env.RemoveServiceEntries(vars["job"])
	writer.WriteHeader(http.StatusOK)
}
