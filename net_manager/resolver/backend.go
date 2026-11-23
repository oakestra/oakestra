package resolver

import (
	"NetManager/TableEntryCache"
	"NetManager/logger"
	"net"
	"net/netip"
)

// Backend answers table queries in place of the cluster's MQTT round trip. In
// p2p mode it is the gossip-replicated service registry, which also keeps track
// of the interests registered through it.
type Backend interface {
	// Resolve returns the table entries for a Service IP or a job name; an
	// empty result means the target is unknown.
	Resolve(target string) ([]TableEntryCache.TableEntry, error)
	RegisterInterest(target string)
	Interests() []string
}

// UseBackend routes cold resolution and interest registration through b
// instead of MQTT. Call it before the proxy starts reading packets.
func (r *ServiceResolver) UseBackend(b Backend) {
	r.tableQuery = func(addr netip.Addr) ([]TableEntryCache.TableEntry, error) {
		return b.Resolve(addr.String())
	}
	r.interestRegistrar = b.RegisterInterest
}

// SetLocalNode tells the translation table which node it runs on, so a
// namespace IP shared with another node's instance resolves to the local one.
func (r *ServiceResolver) SetLocalNode(ip net.IP) {
	r.translationTable.SetLocalNode(ip)
}

// RefreshRoutes re-resolves through b every Service IP this node routes to - its
// registered interests plus the round-robin addresses already in the table - and
// swaps in each job's current instances.
//
// The cluster pushes route updates over MQTT; a backend without push needs
// this to be called periodically, or a replica added after a Service IP was
// first resolved never joins the rotation and a removed one keeps being picked.
// Established flows keep their replica as long as it is still listed.
func (r *ServiceResolver) RefreshRoutes(b Backend) {
	targets := make(map[string]struct{})
	for _, interest := range b.Interests() {
		targets[interest] = struct{}{}
	}
	for _, entry := range r.translationTable.Snapshot() {
		for _, sip := range entry.ServiceIP {
			if sip.IpType == TableEntryCache.RoundRobin && sip.Address != nil {
				targets[sip.Address.String()] = struct{}{}
			}
		}
	}
	for target := range targets {
		resolved, err := b.Resolve(target)
		if err != nil || len(resolved) == 0 {
			continue
		}
		byJob := make(map[string][]TableEntryCache.TableEntry)
		for _, entry := range resolved {
			byJob[entry.JobName] = append(byJob[entry.JobName], entry)
		}
		for job, entries := range byJob {
			if err := r.translationTable.ReplaceJobEntries(job, entries); err != nil {
				logger.ErrorLogger().Printf("route refresh for %s: %v", job, err)
			}
		}
	}
}

// ReloadJob replaces a job's rows with what b currently knows about it.
func (r *ServiceResolver) ReloadJob(b Backend, job string) {
	entries, err := b.Resolve(job)
	if err != nil {
		logger.ErrorLogger().Printf("reload of %s: %v", job, err)
		return
	}
	if err := r.translationTable.ReplaceJobEntries(job, entries); err != nil {
		logger.ErrorLogger().Printf("reload of %s: %v", job, err)
	}
}
