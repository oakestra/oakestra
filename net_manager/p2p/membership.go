package p2p

import (
	"NetManager/logger"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/hashicorp/memberlist"
)

// MembershipConfig configures the SWIM gossip layer.
type MembershipConfig struct {
	NodeUUID   string
	BindAddr   string // "" -> 0.0.0.0
	GossipPort int    // default 50104
	// SecretKey is the gossip PSK (16/24/32 bytes). Encrypts + authenticates all gossip;
	// nodes without it cannot join. nil disables encryption (dev/test only).
	SecretKey []byte
	Seeds     []string // static bootstrap peers "ip:port"
	// OnNodeLeave is invoked when SWIM declares a member dead/left, with the entries that
	// node hosted — this drives failover rescheduling.
	OnNodeLeave func(nodeUUID string, lost []RegistryEntry)
	// OnNodeJoin is invoked when a new member joins — used to retry PENDING workloads
	// when capacity appears.
	OnNodeJoin func(nodeUUID string)
}

// MemberInfo describes one live member of the mesh.
type MemberInfo struct {
	UUID string `json:"uuid"`
	Addr string `json:"addr"` // gossip endpoint ip:port; the node IP hosts all other ports too
}

// DeriveGossipKey turns an arbitrary secret/passphrase into a 32-byte memberlist key.
func DeriveGossipKey(secret string) []byte {
	if secret == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// Membership owns the memberlist instance and bridges it to the service Registry.
type Membership struct {
	ml      *memberlist.Memberlist
	reg     *Registry
	cfg     MembershipConfig
	keyring *memberlist.Keyring
}

// StartMembership starts gossip and joins any configured seeds. The registry is replicated
// via memberlist push/pull anti-entropy (LocalState/MergeRemoteState).
func StartMembership(cfg MembershipConfig, reg *Registry) (*Membership, error) {
	if cfg.GossipPort == 0 {
		cfg.GossipPort = 50104
	}
	m := &Membership{reg: reg, cfg: cfg}

	mlCfg := memberlist.DefaultLANConfig()
	mlCfg.Name = cfg.NodeUUID
	if cfg.BindAddr != "" {
		mlCfg.BindAddr = cfg.BindAddr
	}
	mlCfg.BindPort = cfg.GossipPort
	mlCfg.AdvertisePort = cfg.GossipPort
	if cfg.SecretKey != nil {
		// Explicit keyring (instead of SecretKey) so the PSK can be ROTATED at runtime
		// on revocation without restarting gossip.
		kr, err := memberlist.NewKeyring(nil, cfg.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("p2p: keyring init failed: %w", err)
		}
		mlCfg.Keyring = kr
		m.keyring = kr
	}
	mlCfg.Delegate = &registryDelegate{reg: reg}
	mlCfg.Events = &eventDelegate{m: m}
	// Route memberlist's noisy logging to our debug logger.
	mlCfg.Logger = log.New(io.Discard, "", 0)

	ml, err := memberlist.Create(mlCfg)
	if err != nil {
		return nil, fmt.Errorf("p2p: memberlist create failed: %w", err)
	}
	m.ml = ml

	if len(cfg.Seeds) > 0 {
		n, err := ml.Join(cfg.Seeds)
		if err != nil {
			logger.ErrorLogger().Printf("p2p: join seeds %v: %v (continuing, may re-converge later)", cfg.Seeds, err)
		} else {
			logger.InfoLogger().Printf("p2p: joined %d seed(s), %d members", n, ml.NumMembers())
		}
	} else {
		logger.InfoLogger().Printf("p2p: started gossip on :%d (no seeds — genesis or waiting for discovery)", cfg.GossipPort)
	}

	// Background TTL pruning of expired registry entries.
	go m.pruneLoop()
	return m, nil
}

func (m *Membership) pruneLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		m.reg.PruneExpired(time.Now().Unix())
	}
}

// NumMembers returns the current live membership size (incl. self).
func (m *Membership) NumMembers() int {
	if m.ml == nil {
		return 1
	}
	return m.ml.NumMembers()
}

// Members returns UUID + address of all live members (incl. self).
func (m *Membership) Members() []MemberInfo {
	if m.ml == nil {
		return []MemberInfo{{UUID: m.cfg.NodeUUID}}
	}
	out := make([]MemberInfo, 0)
	for _, n := range m.ml.Members() {
		out = append(out, MemberInfo{UUID: n.Name, Addr: n.Address()})
	}
	return out
}

// MemberUUIDs returns the node names (UUIDs) of all live members.
func (m *Membership) MemberUUIDs() []string {
	if m.ml == nil {
		return []string{m.cfg.NodeUUID}
	}
	out := make([]string, 0)
	for _, n := range m.ml.Members() {
		out = append(out, n.Name)
	}
	return out
}

// JoinLater joins additional seeds after startup (e.g. from mDNS discovery).
func (m *Membership) JoinLater(seeds []string) {
	if m.ml == nil || len(seeds) == 0 {
		return
	}
	if n, err := m.ml.Join(seeds); err != nil {
		logger.ErrorLogger().Printf("p2p: late join %v failed: %v", seeds, err)
	} else if n > 0 {
		logger.InfoLogger().Printf("p2p: joined %d peer(s) via discovery", n)
	}
}

// RotateKey swaps the gossip PSK at runtime (revocation): install the new
// key, make it primary, drop the old one. A revoked node holding only the old key can
// then neither decrypt nor inject gossip.
func (m *Membership) RotateKey(newKey []byte) error {
	if m.keyring == nil {
		return fmt.Errorf("gossip encryption not enabled")
	}
	old := m.keyring.GetPrimaryKey()
	if err := m.keyring.AddKey(newKey); err != nil {
		return err
	}
	if err := m.keyring.UseKey(newKey); err != nil {
		return err
	}
	if old != nil {
		if err := m.keyring.RemoveKey(old); err != nil {
			logger.ErrorLogger().Printf("p2p: could not drop old gossip key: %v", err)
		}
	}
	logger.InfoLogger().Println("p2p: gossip PSK rotated 🔒")
	return nil
}

// Leave gracefully broadcasts an intentional leave (distinct from a crash).
func (m *Membership) Leave(timeout time.Duration) error {
	if m.ml == nil {
		return nil
	}
	return m.ml.Leave(timeout)
}

// Shutdown stops gossip immediately.
func (m *Membership) Shutdown() error {
	if m.ml == nil {
		return nil
	}
	return m.ml.Shutdown()
}

// registryDelegate replicates the registry via memberlist anti-entropy.
type registryDelegate struct{ reg *Registry }

func (d *registryDelegate) NodeMeta(limit int) []byte                  { return nil }
func (d *registryDelegate) NotifyMsg([]byte)                           {}
func (d *registryDelegate) GetBroadcasts(overhead, limit int) [][]byte { return nil }

// LocalState is our full registry view, shared on push/pull sync.
func (d *registryDelegate) LocalState(join bool) []byte { return d.reg.Snapshot() }

// MergeRemoteState folds a peer's registry into ours (LWW by version).
func (d *registryDelegate) MergeRemoteState(buf []byte, join bool) {
	if changed := d.reg.Merge(buf); changed > 0 {
		logger.DebugLogger().Printf("p2p: merged %d registry updates from peer", changed)
	}
}

// eventDelegate reacts to membership changes.
type eventDelegate struct{ m *Membership }

func (e *eventDelegate) NotifyJoin(n *memberlist.Node) {
	logger.InfoLogger().Printf("p2p: member joined %s (%s)", n.Name, n.Address())
	if e.m.cfg.OnNodeJoin != nil && n.Name != e.m.cfg.NodeUUID {
		go e.m.cfg.OnNodeJoin(n.Name)
	}
}

func (e *eventDelegate) NotifyLeave(n *memberlist.Node) {
	logger.InfoLogger().Printf("p2p: member left %s — reconciling its services", n.Name)
	lost := e.m.reg.DeleteByNode(n.Name)
	if e.m.cfg.OnNodeLeave != nil {
		e.m.cfg.OnNodeLeave(n.Name, lost)
	}
}

func (e *eventDelegate) NotifyUpdate(n *memberlist.Node) {}
