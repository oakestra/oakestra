package p2p

import (
	"NetManager/TableEntryCache"
	"NetManager/logger"
	"sync"
	"time"
)

// ServiceResolver abstracts service discovery so the NetManager environment can use either
// the cluster MQTT table query or the p2p registry behind one interface.
type ServiceResolver interface {
	// Resolve returns table entries for a ServiceIP or job name. In p2p mode this is a
	// local map lookup with an on-demand gossip search fallback.
	Resolve(sipOrName string) ([]TableEntryCache.TableEntry, error)
	// RegisterInterest marks a route this node cares about (kept until unused).
	RegisterInterest(jobName string)
	DeregisterInterest(jobName string)
	// Interests lists what this node currently cares about, so routes can be refreshed.
	Interests() []string
	// Advertise publishes a locally-hosted service instance into the registry.
	Advertise(entry RegistryEntry)
	// Withdraw removes a locally-hosted instance (e.g. on undeploy).
	Withdraw(jobName string, instance int)
}

// Searcher performs an on-demand query for a ServiceIP over gossip. It is
// optional; when nil, Resolve serves purely from the replicated registry.
type Searcher interface {
	QueryServiceIP(ip string) []RegistryEntry
}

// registryResolver is the p2p ServiceResolver: reads served from the replicated Registry,
// with an optional on-demand Searcher fallback for not-yet-converged entries.
type registryResolver struct {
	reg      *Registry
	nodeUUID string
	search   Searcher

	mu        sync.Mutex
	interests map[string]struct{}
}

// NewResolver builds a p2p ServiceResolver over the given registry. search may be nil.
func NewResolver(reg *Registry, nodeUUID string, search Searcher) ServiceResolver {
	return &registryResolver{
		reg:       reg,
		nodeUUID:  nodeUUID,
		search:    search,
		interests: make(map[string]struct{}),
	}
}

func (r *registryResolver) Resolve(sipOrName string) ([]TableEntryCache.TableEntry, error) {
	// Try as a ServiceIP first, then as a job name (matches the MQTT resolver semantics).
	entries := r.reg.TableEntriesForServiceIP(sipOrName)
	if len(entries) == 0 {
		entries = r.reg.TableEntriesForJob(sipOrName)
	}
	if len(entries) > 0 {
		return entries, nil
	}

	// Miss: fall back to an on-demand p2p search.
	if r.search != nil {
		found := r.search.QueryServiceIP(sipOrName)
		for _, e := range found {
			r.reg.Merge(mustJSON([]RegistryEntry{e}))
		}
		if len(found) > 0 {
			return r.reg.TableEntriesForServiceIP(sipOrName), nil
		}
	}
	return []TableEntryCache.TableEntry{}, nil
}

func (r *registryResolver) RegisterInterest(jobName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interests[jobName] = struct{}{}
}

func (r *registryResolver) DeregisterInterest(jobName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.interests, jobName)
}

func (r *registryResolver) Interests() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.interests))
	for name := range r.interests {
		out = append(out, name)
	}
	return out
}

func (r *registryResolver) Advertise(entry RegistryEntry) {
	if entry.NodeUUID == "" {
		entry.NodeUUID = r.nodeUUID
	}
	r.reg.Put(entry)
	logger.InfoLogger().Printf("p2p: advertised %s (node %s, %d instances)", entry.JobName, entry.NodeUUID, len(entry.Instances))
}

// TombstoneTTL is how long a withdrawn entry lingers as an empty, versioned record
// before being pruned. It must comfortably exceed the gossip push/pull period so every
// member sees the removal; the entry costs a few hundred bytes until then.
const TombstoneTTL = 10 * time.Minute

func (r *registryResolver) Withdraw(jobName string, instance int) {
	for _, e := range r.reg.FindByJob(jobName) {
		if e.NodeUUID != r.nodeUUID {
			continue
		}
		kept := make([]InstanceEntry, 0, len(e.Instances))
		for _, inst := range e.Instances {
			if inst.InstanceNumber != instance {
				kept = append(kept, inst)
			}
		}
		e.Instances = kept
		e.Version++ // force propagation of the removal

		if len(e.Instances) == 0 {
			// Deleting the key outright would not propagate: Merge is last-writer-wins on
			// Version and has no notion of absence, so the next anti-entropy round with a
			// peer that still holds the entry would simply put it back. Publishing an
			// empty, higher-versioned record is the removal; the TTL sweeps it up later.
			e.TTLDeadline = TTLFrom(time.Now(), TombstoneTTL)
		}
		r.reg.Put(e)
	}
}

func mustJSON(v []RegistryEntry) []byte {
	b := NewRegistry()
	for _, e := range v {
		b.entries[regKey(e.JobName, e.NodeUUID)] = e
	}
	return b.Snapshot()
}
