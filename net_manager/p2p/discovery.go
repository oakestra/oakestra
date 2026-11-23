package p2p

import (
	"NetManager/logger"
	"fmt"
	"time"

	"github.com/hashicorp/mdns"
)

// mDNS service tag advertised/browsed on the LAN.
const mdnsService = "_oakestra-p2p._udp"

// Discovery finds peer gossip addresses. Trust is NOT granted by discovery — the gossip
// PSK + roster gate who a node will actually talk to, so discovery can be open.
type Discovery interface {
	// Advertise announces this node's gossip endpoint on the LAN.
	Advertise(nodeUUID string, gossipPort int) error
	// Browse returns currently-discovered peer "ip:port" gossip addresses.
	Browse(timeout time.Duration) []string
	// Close stops advertising.
	Close() error
}

// mDNSDiscovery uses zeroconf/mDNS for zero-config LAN discovery. Used mainly for
// stale-seed recovery on re-join — not first-contact trust.
type mDNSDiscovery struct {
	server *mdns.Server
}

func NewMDNSDiscovery() Discovery { return &mDNSDiscovery{} }

func (d *mDNSDiscovery) Advertise(nodeUUID string, gossipPort int) error {
	info := []string{"oakestra p2p node " + nodeUUID}
	svc, err := mdns.NewMDNSService(nodeUUID, mdnsService, "", "", gossipPort, nil, info)
	if err != nil {
		return fmt.Errorf("mdns service: %w", err)
	}
	server, err := mdns.NewServer(&mdns.Config{Zone: svc})
	if err != nil {
		return fmt.Errorf("mdns server: %w", err)
	}
	d.server = server
	logger.InfoLogger().Printf("p2p: advertising via mDNS as %s on :%d", nodeUUID, gossipPort)
	return nil
}

func (d *mDNSDiscovery) Browse(timeout time.Duration) []string {
	entries := make(chan *mdns.ServiceEntry, 16)
	peers := make([]string, 0)
	done := make(chan struct{})
	go func() {
		for e := range entries {
			if e.AddrV4 != nil {
				peers = append(peers, fmt.Sprintf("%s:%d", e.AddrV4.String(), e.Port))
			} else if e.AddrV6 != nil {
				peers = append(peers, fmt.Sprintf("[%s]:%d", e.AddrV6.String(), e.Port))
			}
		}
		close(done)
	}()

	params := mdns.DefaultParams(mdnsService)
	params.Entries = entries
	params.Timeout = timeout
	params.DisableIPv6 = false
	_ = mdns.Query(params)
	close(entries)
	<-done
	return peers
}

func (d *mDNSDiscovery) Close() error {
	if d.server != nil {
		return d.server.Shutdown()
	}
	return nil
}

// StaticDiscovery is a no-op advertiser that simply returns the configured seed peers.
type StaticDiscovery struct{ Seeds []string }

func (s StaticDiscovery) Advertise(string, int) error   { return nil }
func (s StaticDiscovery) Browse(time.Duration) []string { return s.Seeds }
func (s StaticDiscovery) Close() error                  { return nil }
