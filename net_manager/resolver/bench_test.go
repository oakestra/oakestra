package resolver

import (
	"NetManager/TableEntryCache"
	"net"
	"net/netip"
	"testing"
)

// The proxy package's datapath benchmarks read the TableManager directly, so
// they miss the Touch() that keeps a job's MQTT interest alive. These go
// through the real resolver to cover that too.

func benchResolver(b *testing.B, entries ...TableEntryCache.TableEntry) *ServiceResolver {
	b.Helper()
	r := &ServiceResolver{
		translationTable: TableEntryCache.NewTableManager(),
		// a miss must not reach MQTT; every benchmark below is a hit anyway
		tableQuery: func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
			b.Fatal("benchmark hit the resolution path; the table should already hold the entry")
			return nil, nil
		},
		interestRegistrar: func(string) {},
	}
	for _, entry := range entries {
		if err := r.translationTable.Add(entry); err != nil {
			b.Fatal(err)
		}
	}
	return r
}

// benchEntry adds the InstanceNumber ServiceIP that GetInstanceIP needs;
// resolvableEntry's RoundRobin one alone would make that lookup miss.
func benchEntry(job, nsip, vip string) TableEntryCache.TableEntry {
	entry := resolvableEntry(job, vip)
	entry.Nsip = net.ParseIP(nsip)
	entry.ServiceIP = append(entry.ServiceIP, TableEntryCache.ServiceIP{
		IpType:     TableEntryCache.InstanceNumber,
		Address:    net.ParseIP("10.30.100.1"),
		Address_v6: net.ParseIP("fdff::100"),
	})
	return entry
}

// BenchmarkGetTableEntryByServiceIP is the outgoing path's first lookup. It
// includes the per-packet Touch() that keeps the job's MQTT interest alive.
func BenchmarkGetTableEntryByServiceIP(b *testing.B) {
	const vip = "10.30.1.1"
	r := benchResolver(b, benchEntry("job", "10.19.1.1", vip))
	addr := netip.MustParseAddr(vip)

	b.ReportAllocs()
	for b.Loop() {
		if lookup := r.GetTableEntryByServiceIP(addr); len(lookup.Entries) != 1 {
			b.Fatal("expected the entry to resolve from the local table")
		}
	}
}

// BenchmarkGetInstanceIP is the outgoing path's second lookup: the source
// namespace IP to the instance address the packet is re-sourced from.
func BenchmarkGetInstanceIP(b *testing.B) {
	r := benchResolver(b, benchEntry("job", "10.19.1.1", "10.30.1.1"))
	addr := netip.MustParseAddr("10.19.1.1")

	b.ReportAllocs()
	for b.Loop() {
		if _, ok := r.GetInstanceIP(addr, 4); !ok {
			b.Fatal("expected the instance IP to resolve from the local table")
		}
	}
}
