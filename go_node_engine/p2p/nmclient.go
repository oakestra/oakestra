package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go_node_engine/model"
	"io"
	"net"
	"net/http"
	"time"
)

// Registry wire types — must stay JSON-compatible with NetManager/p2p (oakestra-net).

type ServiceIPEntry struct {
	Type      string `json:"type"` // "RR" | "Closest" | "InstanceNumber"
	Address   string `json:"address"`
	AddressV6 string `json:"address_v6"`
}

type InstanceEntry struct {
	InstanceNumber int              `json:"instance_number"`
	Status         string           `json:"status"`
	NamespaceIP    string           `json:"namespace_ip"`
	NamespaceIPv6  string           `json:"namespace_ip_v6"`
	HostIP         string           `json:"host_ip"`
	HostPort       int              `json:"host_port"`
	ServiceIP      []ServiceIPEntry `json:"service_ip"`
}

type RegistryEntry struct {
	JobName     string          `json:"job_name"`
	NodeUUID    string          `json:"node_uuid"`
	Generation  int             `json:"generation"`
	Instances   []InstanceEntry `json:"instances"`
	Version     uint64          `json:"version"`
	TTLDeadline int64           `json:"ttl_deadline"`
	Meta        string          `json:"meta,omitempty"`
}

type MemberInfo struct {
	UUID string `json:"uuid"`
	Addr string `json:"addr"` // gossip endpoint ip:port
}

// IP returns the member's bare IP (without the gossip port).
func (m MemberInfo) IP() string {
	host, _, err := net.SplitHostPort(m.Addr)
	if err != nil {
		return m.Addr
	}
	return host
}

// NMClient talks to the paired NetManager's p2p endpoints over the same local
// connection the NodeEngine already uses (unix socket or localhost port).
type NMClient struct {
	http *http.Client
	base string
}

// NewNMClient builds the client from the node's NetManager connection settings.
func NewNMClient() *NMClient {
	port := model.GetNodeInfo().NetManagerPort
	client := &http.Client{Timeout: 10 * time.Second}
	if port == 0 {
		socket := model.GetNodeInfo().OverlaySocket
		client.Transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socket)
			},
		}
	}
	return &NMClient{http: client, base: fmt.Sprintf("http://localhost:%d", port)}
}

func (c *NMClient) get(path string, out interface{}) error {
	resp, err := c.http.Get(c.base + path)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NetManager %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// Members returns the current live mesh membership.
func (c *NMClient) Members() ([]MemberInfo, error) {
	var members []MemberInfo
	err := c.get("/p2p/members", &members)
	return members, err
}

// Services returns the full gossiped registry (network-wide view, plan §8).
func (c *NMClient) Services() ([]RegistryEntry, error) {
	var entries []RegistryEntry
	err := c.get("/p2p/services", &entries)
	return entries, err
}

// Advertise publishes a locally-hosted service into the gossip registry (plan §2.4).
func (c *NMClient) Advertise(entry RegistryEntry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.base+"/p2p/service", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("advertise failed: status %d", resp.StatusCode)
	}
	return nil
}

// Rotate tells the local NetManager to swap the gossip PSK + data key (plan §9.6).
func (c *NMClient) Rotate(gossipKey string) error {
	body, _ := json.Marshal(map[string]string{"gossip_key": gossipKey})
	resp, err := c.http.Post(c.base+"/p2p/rotate", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rotate failed: status %d", resp.StatusCode)
	}
	return nil
}

// Withdraw removes a local instance from the gossip registry.
func (c *NMClient) Withdraw(jobName string, instance int) error {
	req, err := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/p2p/service/%s/%d", c.base, jobName, instance), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("withdraw failed: status %d", resp.StatusCode)
	}
	return nil
}
