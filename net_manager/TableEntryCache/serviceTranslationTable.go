package TableEntryCache

import (
	"NetManager/events"
	"NetManager/logger"
	"errors"
	"log"
	"net"
	"net/netip"
	"regexp"
	"sync"
	"sync/atomic"
)

// compiled once instead of per IsValid call
var nameRegexp = regexp.MustCompile("^[a-zA-Z0-9]{1,30}$")

type TableEntry struct {
	JobName          string      `json:"job_name"`
	Appname          string      `json:"appname"`
	Appns            string      `json:"appns"`
	Servicename      string      `json:"servicename"`
	Servicenamespace string      `json:"servicenamespace"`
	Instancenumber   int         `json:"instancenumber"`
	Cluster          int         `json:"cluster"`
	Nodeip           net.IP      `json:"nodeip"`
	Nodeport         int         `json:"nodeport"`
	Nsip             net.IP      `json:"nsip"`
	Nsipv6           net.IP      `json:"nsipv6"`
	ServiceIP        []ServiceIP `json:"serviceIP"`
	// shared per JobName (set once in Add), so the MQTT interest timer knows
	// when the job was last used without a lock or map lookup per packet.
	activity *events.Activity `json:"-"`
}

// Touch records that this entry's job was just used on the packet path.
func (e *TableEntry) Touch() {
	if e.activity != nil {
		e.activity.Touch()
	}
}

// Activity returns the entry's shared per-job activity stamp, or nil for an
// entry that never went through Add.
func (e *TableEntry) Activity() *events.Activity {
	return e.activity
}

type ServiceIpType int

const (
	InstanceNumber ServiceIpType = iota
	Closest        ServiceIpType = iota
	RoundRobin     ServiceIpType = iota
)

type ServiceIP struct {
	IpType     ServiceIpType `json:"ip_type"`
	Address    net.IP        `json:"address"`
	Address_v6 net.IP        `json:"address_v6"`
}

// InstanceAddrs holds the two addresses that identify one deployed instance,
// without the rest of a TableEntry's job metadata. Copying a whole TableEntry
// per packet just to read these used to dominate the outgoing path.
type InstanceAddrs struct{ V4, V6 netip.Addr }

// For returns the instance address in the requested address family.
func (a InstanceAddrs) For(version uint8) netip.Addr {
	if version == 6 {
		return a.V6
	}
	return a.V4
}

// InstanceAddrsOf returns the addresses that uniquely identify one deployed
// instance of a service: the ones its own proxy sources replies from. Every
// caller has to agree on this, or a route installed under one reading gets
// matched against replies under another.
func InstanceAddrsOf(entry *TableEntry) InstanceAddrs {
	for _, sip := range entry.ServiceIP {
		if sip.IpType != InstanceNumber {
			continue
		}
		var addrs InstanceAddrs
		addrs.V4, _ = AddrFromIP(sip.Address)
		addrs.V6, _ = AddrFromIP(sip.Address_v6)
		return addrs
	}
	return InstanceAddrs{}
}

type TableManager struct {
	translationTable []TableEntry
	// address indexes for SearchByServiceIP/SearchByNsIP; hold copies, not
	// pointers into translationTable, since that slice gets swap-removed and
	// reallocated. Rebuilt wholesale on every mutation.
	byServiceIP map[netip.Addr][]TableEntry
	byNsIP      map[netip.Addr]TableEntry
	// byNsIPInstance precomputes the packet path's only question about a
	// namespace IP, so a lookup is one map read instead of copying a
	// TableEntry and rescanning its ServiceIP slice per packet.
	byNsIPInstance map[netip.Addr]InstanceAddrs
	// generation bumps on every index rebuild. A cached route tagged with the
	// current generation is known still valid, so the packet path can skip
	// rescanning replicas on a hit. It's atomic rather than rwlock-guarded so
	// that check doesn't need the lock at all; see Generation.
	generation atomic.Uint64
	rwlock     sync.RWMutex
}

func NewTableManager() TableManager {
	return TableManager{
		translationTable: make([]TableEntry, 0),
		byServiceIP:      make(map[netip.Addr][]TableEntry),
		byNsIP:           make(map[netip.Addr]TableEntry),
		byNsIPInstance:   make(map[netip.Addr]InstanceAddrs),
		rwlock:           sync.RWMutex{},
	}
	// TODO cleanup of old entry every X seconds
}

// AddrFromIP converts a net.IP to a netip.Addr for use as a map key. The
// Unmap call matters: net.ParseIP stores IPv4 as 16-byte IPv4-in-IPv6, which
// otherwise compares unequal to a plain 4-byte address even though both
// print the same.
func AddrFromIP(ip net.IP) (netip.Addr, bool) {
	if ip == nil {
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// rebuildIndexesLocked recomputes all three address indexes from
// translationTable. Caller must hold rwlock for writing.
func (t *TableManager) rebuildIndexesLocked() {
	byServiceIP := make(map[netip.Addr][]TableEntry, len(t.byServiceIP))
	byNsIP := make(map[netip.Addr]TableEntry, len(t.translationTable))
	byNsIPInstance := make(map[netip.Addr]InstanceAddrs, len(t.translationTable))
	for i, entry := range t.translationTable {
		instance := InstanceAddrsOf(&t.translationTable[i])
		if addr, ok := AddrFromIP(entry.Nsip); ok {
			byNsIP[addr] = entry
			byNsIPInstance[addr] = instance
		}
		if addr, ok := AddrFromIP(entry.Nsipv6); ok {
			byNsIP[addr] = entry
			byNsIPInstance[addr] = instance
		}
		for _, sip := range entry.ServiceIP {
			if addr, ok := AddrFromIP(sip.Address); ok {
				byServiceIP[addr] = append(byServiceIP[addr], entry)
			}
			if addr, ok := AddrFromIP(sip.Address_v6); ok {
				byServiceIP[addr] = append(byServiceIP[addr], entry)
			}
		}
	}
	t.byServiceIP = byServiceIP
	t.byNsIP = byNsIP
	t.byNsIPInstance = byNsIPInstance
	// Published last: a reader that sees this generation must be able to see
	// the indexes it describes.
	t.generation.Add(1)
}

func (t *TableManager) Add(entry TableEntry) error {
	if !IsValid(entry) {
		return errors.New("InvalidEntry")
	}
	entry.activity = events.GetOrCreate(entry.JobName)

	t.rwlock.Lock()
	defer t.rwlock.Unlock()
	t.translationTable = append(t.translationTable, entry)
	t.rebuildIndexesLocked()
	return nil
}

// ReplaceJobEntries swaps every entry belonging to jobName for a new set in a
// single locked mutation, rebuilding the indexes once instead of once per
// entry. Entries are validated before the lock is taken, so a bad input
// leaves the table untouched rather than half-replaced.
func (t *TableManager) ReplaceJobEntries(jobName string, entries []TableEntry) error {
	prepared := make([]TableEntry, 0, len(entries))
	for _, entry := range entries {
		if !IsValid(entry) {
			return errors.New("InvalidEntry")
		}
		entry.activity = events.GetOrCreate(entry.JobName)
		prepared = append(prepared, entry)
	}

	// an incoming entry reusing a namespace IP displaces whoever held it
	replacedNsIPs := make(map[netip.Addr]struct{}, 2*len(prepared))
	for _, entry := range prepared {
		if addr, ok := AddrFromIP(entry.Nsip); ok {
			replacedNsIPs[addr] = struct{}{}
		}
		if addr, ok := AddrFromIP(entry.Nsipv6); ok {
			replacedNsIPs[addr] = struct{}{}
		}
	}

	t.rwlock.Lock()
	defer t.rwlock.Unlock()

	kept := t.translationTable[:0]
	for _, existing := range t.translationTable {
		if existing.JobName == jobName || nsIPClaimed(existing, replacedNsIPs) {
			continue
		}
		kept = append(kept, existing)
	}
	t.translationTable = append(kept, prepared...)
	t.rebuildIndexesLocked()
	return nil
}

func nsIPClaimed(entry TableEntry, claimed map[netip.Addr]struct{}) bool {
	if addr, ok := AddrFromIP(entry.Nsip); ok {
		if _, taken := claimed[addr]; taken {
			return true
		}
	}
	if addr, ok := AddrFromIP(entry.Nsipv6); ok {
		if _, taken := claimed[addr]; taken {
			return true
		}
	}
	return false
}

// remove by Namespace IP, which can be either in IPv4 or IPv6 format
func (t *TableManager) RemoveByNsip(nsip net.IP) error {
	logger.DebugLogger().Printf("Remove by Nsip tableManager: %v", t)

	t.rwlock.Lock()
	defer t.rwlock.Unlock()

	found := -1
	// this will need to be optimised for IPv6, since that will be hell performance wise
	for i, tableElement := range t.translationTable {
		if tableElement.Nsip.Equal(nsip) || tableElement.Nsipv6.Equal(nsip) {
			found = i
			break
		}
	}

	if found < 0 {
		return errors.New("entry not found")
	}
	logger.DebugLogger().Printf("Removing from TableManager: %v", t.translationTable[found])
	t.removeByIndexLocked(found)
	t.rebuildIndexesLocked()
	return nil
}

// RemoveByJobName drops every entry for a job in one pass, rebuilding the
// indexes once at the end.
func (t *TableManager) RemoveByJobName(jobname string) error {
	t.rwlock.Lock()
	defer t.rwlock.Unlock()

	kept := t.translationTable[:0]
	for _, entry := range t.translationTable {
		if entry.JobName == jobname {
			logger.DebugLogger().Printf("Removing from TableManager: %v", entry)
			continue
		}
		kept = append(kept, entry)
	}
	t.translationTable = kept
	t.rebuildIndexesLocked()
	return nil
}

// removeByIndexLocked swap-removes one entry; caller must rebuildIndexesLocked afterwards.
func (t *TableManager) removeByIndexLocked(index int) {
	t.translationTable[index] = t.translationTable[len(t.translationTable)-1]
	t.translationTable = t.translationTable[:len(t.translationTable)-1]
}

// SearchByServiceIP looks up entries by ServiceIP via the index and returns
// the generation the result was read under (see TableManager.generation). The
// slice is capped at its length so a caller append can't spill into the map's
// spare capacity; that's safe because rebuildIndexesLocked always replaces
// byServiceIP wholesale instead of mutating a bucket in place.
func (t *TableManager) SearchByServiceIP(addr netip.Addr) ([]TableEntry, uint64) {
	t.rwlock.RLock()
	defer t.rwlock.RUnlock()
	matches := t.byServiceIP[addr]
	return matches[:len(matches):len(matches)], t.generation.Load()
}

// Generation returns the current index generation without taking the lock.
// A rebuild racing this read just means one more packet takes the old route,
// the same window a locked read would have anyway.
func (t *TableManager) Generation() uint64 {
	return t.generation.Load()
}

// SearchByNsIP looks up a single entry by namespace IP via the index.
func (t *TableManager) SearchByNsIP(addr netip.Addr) (TableEntry, bool) {
	t.rwlock.RLock()
	defer t.rwlock.RUnlock()
	entry, found := t.byNsIP[addr]
	return entry, found
}

// SearchInstanceIPByNsIP resolves the instance address that identifies addr's
// own service instance, in the requested address family. This is the packet
// path's lookup: unlike SearchByNsIP it copies one address out of the index
// rather than a whole TableEntry.
func (t *TableManager) SearchInstanceIPByNsIP(addr netip.Addr, version uint8) (netip.Addr, bool) {
	t.rwlock.RLock()
	instance, found := t.byNsIPInstance[addr]
	t.rwlock.RUnlock()
	if !found {
		return netip.Addr{}, false
	}
	result := instance.For(version)
	return result, result.IsValid()
}

func (t *TableManager) SearchByNodeIp(ip net.IP) []TableEntry {
	result := make([]TableEntry, 0)
	t.rwlock.RLock()
	defer t.rwlock.RUnlock()
	for _, tableElement := range t.translationTable {
		if tableElement.Nodeip.Equal(ip) {
			result = append(result, tableElement)
		}
	}
	return result
}

func (t *TableManager) SearchByJobName(jobname string) []TableEntry {
	t.rwlock.RLock()
	defer t.rwlock.RUnlock()
	results := make([]TableEntry, 0)
	for _, tableElement := range t.translationTable {
		if tableElement.JobName == jobname {
			results = append(results, tableElement)
		}
	}
	return results
}

// IsValid is the sanity check an entry must pass to enter the table:
// Appname, Appns, Servicename and Servicenamespace match nameRegexp
// Instancenumber>=0
// Cluster>=0
// Nodeip != nil
// Nsip != nil
// Nsipv6 != nil
// len(entry.ServiceIP)>0
func IsValid(entry TableEntry) bool {
	if !nameRegexp.MatchString(entry.Appname) {
		log.Println("TranslationTable: Invalid Entry, wrong appname:", entry.Appname)
		return false
	}
	if !nameRegexp.MatchString(entry.Appns) {
		log.Println("TranslationTable: Invalid Entry, wrong appns:", entry.Appns)
		return false
	}
	if !nameRegexp.MatchString(entry.Servicename) {
		log.Println("TranslationTable: Invalid Entry, wrong servicename:", entry.Servicename)
		return false
	}
	if !nameRegexp.MatchString(entry.Servicenamespace) {
		log.Println("TranslationTable: Invalid Entry, wrong servicens:", entry.Servicenamespace)
		return false
	}
	if entry.Instancenumber < 0 {
		log.Println("TranslationTable: Invalid Entry, wrong instancenumber")
		return false
	}
	if entry.Cluster < 0 {
		log.Println("TranslationTable: Invalid Entry, wrong cluster")
		return false
	}
	if entry.Nodeip == nil {
		log.Println("TranslationTable: Invalid Entry, wrong nodeip")
		return false
	}
	if entry.Nsip == nil {
		log.Println("TranslationTable: Invalid Entry, wrong nsip")
		return false
	}
	if entry.Nsipv6 == nil {
		log.Println("TranslationTable: Invalid Entry, wrong nsipv6")
		return false
	}
	if len(entry.ServiceIP) < 1 {
		log.Println("TranslationTable: Invalid Entry, wrong serviceip")
		return false
	}
	return true
}

// MatchRoute finds the entry a cached route is pinned to, matching the full
// route (nsip, node IP, node port) rather than just nsip. A refresh can
// reassign an instance to a different node while its nsip stays the same, so
// an nsip-only check would keep a cached flow tunnelling to a stale node
// forever. Returns nil if the instance is gone, so the caller picks a new one.
//
// It hands back the entry itself, not just a bool, because a route that
// survives a table rebuild still needs refreshing from it: the job's activity
// stamp is replaced if the job was removed and re-added, and the address its
// replies come from can change too.
func MatchRoute(nsip netip.Addr, nodeip netip.Addr, nodeport int, table []TableEntry) *TableEntry {
	for i := range table {
		entry := &table[i]
		if entry.Nodeport != nodeport {
			continue
		}
		entryNodeip, ok := AddrFromIP(entry.Nodeip)
		if !ok || entryNodeip != nodeip {
			continue
		}
		if entryNsip, ok := AddrFromIP(entry.Nsip); ok && entryNsip == nsip {
			return entry
		}
		if entryNsipv6, ok := AddrFromIP(entry.Nsipv6); ok && entryNsipv6 == nsip {
			return entry
		}
	}
	return nil
}
