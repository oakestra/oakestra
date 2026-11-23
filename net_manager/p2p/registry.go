// Package p2p implements the peer-to-peer overlay for standalone (clusterless) workers:
// SWIM gossip membership (via hashicorp/memberlist), an eventually-consistent service
// registry replicated by anti-entropy, and a ServiceResolver used by the NetManager in
// place of the cluster MQTT table query.
package p2p

import (
	"NetManager/TableEntryCache"
	"encoding/json"
	"net"
	"sync"
	"time"
)

// ServiceIPEntry mirrors a TableEntryCache.ServiceIP in a JSON-friendly form for gossip.
type ServiceIPEntry struct {
	Type      string `json:"type"` // "RR" | "Closest" | "InstanceNumber"
	Address   string `json:"address"`
	AddressV6 string `json:"address_v6"`
}

// InstanceEntry is one running instance of a service.
type InstanceEntry struct {
	InstanceNumber int              `json:"instance_number"`
	Status         string           `json:"status"` // RUNNING|DEAD|FAILED|COMPLETED
	NamespaceIP    string           `json:"namespace_ip"`
	NamespaceIPv6  string           `json:"namespace_ip_v6"`
	HostIP         string           `json:"host_ip"`
	HostPort       int              `json:"host_port"`
	ServiceIP      []ServiceIPEntry `json:"service_ip"`
}

// RegistryEntry is the gossiped record for a (job, hosting node). Keyed by JobName+NodeUUID.
type RegistryEntry struct {
	JobName     string          `json:"job_name"`
	NodeUUID    string          `json:"node_uuid"`
	Generation  int             `json:"generation"` // reboot/failover tie-break
	Instances   []InstanceEntry `json:"instances"`
	Version     uint64          `json:"version"`      // monotonic per node for LWW
	TTLDeadline int64           `json:"ttl_deadline"` // unix seconds; 0 = no expiry
	// Meta carries non-service gossip payloads (e.g. "__roster__" cert fingerprints).
	Meta string `json:"meta,omitempty"`
}

func regKey(jobName, nodeUUID string) string { return jobName + "|" + nodeUUID }

// Registry is the local view of the network-wide service registry. It is replicated by
// anti-entropy (Snapshot/Merge) so every node converges on the same map.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]RegistryEntry // regKey -> entry
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]RegistryEntry)}
}

// Put inserts/updates a locally-originated entry, bumping its version.
func (r *Registry) Put(e RegistryEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := regKey(e.JobName, e.NodeUUID)
	if prev, ok := r.entries[k]; ok && e.Version <= prev.Version {
		e.Version = prev.Version + 1
	}
	if e.Version == 0 {
		e.Version = 1
	}
	r.entries[k] = e
}

// Delete removes an entry for a (job, node).
func (r *Registry) Delete(jobName, nodeUUID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, regKey(jobName, nodeUUID))
}

// DeleteByNode removes all entries hosted by a node (used on NodeLeave).
func (r *Registry) DeleteByNode(nodeUUID string) []RegistryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := make([]RegistryEntry, 0)
	for k, e := range r.entries {
		if e.NodeUUID == nodeUUID {
			removed = append(removed, e)
			delete(r.entries, k)
		}
	}
	return removed
}

// FindByJob returns all entries (across hosting nodes) for a job name.
func (r *Registry) FindByJob(jobName string) []RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RegistryEntry, 0)
	for _, e := range r.entries {
		if e.JobName == jobName {
			out = append(out, e)
		}
	}
	return out
}

// FindByServiceIP returns all entries offering the given 10.30.x.x ServiceIP.
func (r *Registry) FindByServiceIP(ip string) []RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RegistryEntry, 0)
	for _, e := range r.entries {
		if entryHasServiceIP(e, ip) {
			out = append(out, e)
		}
	}
	return out
}

func entryHasServiceIP(e RegistryEntry, ip string) bool {
	for _, inst := range e.Instances {
		for _, sip := range inst.ServiceIP {
			if sip.Address == ip || sip.AddressV6 == ip {
				return true
			}
		}
	}
	return false
}

// Snapshot serialises the whole registry for anti-entropy (memberlist LocalState).
func (r *Registry) Snapshot() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]RegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		list = append(list, e)
	}
	b, _ := json.Marshal(list)
	return b
}

// Merge applies a remote snapshot, keeping the higher (Version) per key (LWW). Returns
// the number of entries that were newly added or updated.
func (r *Registry) Merge(buf []byte) int {
	var incoming []RegistryEntry
	if err := json.Unmarshal(buf, &incoming); err != nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := 0
	for _, e := range incoming {
		k := regKey(e.JobName, e.NodeUUID)
		if prev, ok := r.entries[k]; !ok || e.Version > prev.Version {
			r.entries[k] = e
			changed++
		}
	}
	return changed
}

// PruneExpired drops entries whose TTL has passed. Returns removed entries.
func (r *Registry) PruneExpired(now int64) []RegistryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := make([]RegistryEntry, 0)
	for k, e := range r.entries {
		if e.TTLDeadline != 0 && now > e.TTLDeadline {
			removed = append(removed, e)
			delete(r.entries, k)
		}
	}
	return removed
}

// TableEntriesForJob converts all registry entries for a job into the TableEntry shape the
// NetManager proxy consumes (identical to the MQTT table-query result).
func (r *Registry) TableEntriesForJob(jobName string) []TableEntryCache.TableEntry {
	return toTableEntries(r.FindByJob(jobName))
}

// TableEntriesForServiceIP converts all registry entries offering an IP into TableEntries.
func (r *Registry) TableEntriesForServiceIP(ip string) []TableEntryCache.TableEntry {
	return toTableEntries(r.FindByServiceIP(ip))
}

func toTableEntries(entries []RegistryEntry) []TableEntryCache.TableEntry {
	out := make([]TableEntryCache.TableEntry, 0)
	for _, e := range entries {
		app := splitJobName(e.JobName)
		for _, inst := range e.Instances {
			sips := make([]TableEntryCache.ServiceIP, 0, len(inst.ServiceIP))
			for _, s := range inst.ServiceIP {
				sips = append(sips, TableEntryCache.ServiceIP{
					IpType:     serviceIPType(s.Type),
					Address:    net.ParseIP(s.Address),
					Address_v6: net.ParseIP(s.AddressV6),
				})
			}
			out = append(out, TableEntryCache.TableEntry{
				JobName:          e.JobName,
				Appname:          app[0],
				Appns:            app[1],
				Servicename:      app[2],
				Servicenamespace: app[3],
				Instancenumber:   inst.InstanceNumber,
				Cluster:          0,
				Nodeip:           net.ParseIP(inst.HostIP),
				Nodeport:         inst.HostPort,
				Nsip:             net.ParseIP(inst.NamespaceIP),
				Nsipv6:           net.ParseIP(inst.NamespaceIPv6),
				ServiceIP:        sips,
			})
		}
	}
	return out
}

func serviceIPType(t string) TableEntryCache.ServiceIpType {
	switch t {
	case "RR":
		return TableEntryCache.RoundRobin
	case "Closest":
		return TableEntryCache.Closest
	case "InstanceNumber":
		return TableEntryCache.InstanceNumber
	default:
		return TableEntryCache.RoundRobin
	}
}

// splitJobName returns the 4 components of app.appns.service.servicens, padding if malformed.
func splitJobName(jobName string) [4]string {
	var out [4]string
	parts := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(jobName); i++ {
		if jobName[i] == '.' {
			parts = append(parts, jobName[start:i])
			start = i + 1
		}
	}
	parts = append(parts, jobName[start:])
	for i := 0; i < 4 && i < len(parts); i++ {
		out[i] = parts[i]
	}
	return out
}

// TTLFrom returns a deadline `d` from now, as unix seconds.
func TTLFrom(now time.Time, d time.Duration) int64 { return now.Add(d).Unix() }
