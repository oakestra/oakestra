package TableEntryCache

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"testing"
)

// spreads across octets so instance counts above 254 still parse
func jobNsIP(instance int) string {
	return fmt.Sprintf("10.19.%d.%d", instance/250+1, instance%250+1)
}

func jobEntry(job string, instance int, nodeip string, nodeport int) TableEntry {
	return TableEntry{
		JobName:          job,
		Appname:          "app",
		Appns:            "ns",
		Servicename:      "svc",
		Servicenamespace: "svcns",
		Instancenumber:   instance,
		Cluster:          0,
		Nodeip:           net.ParseIP(nodeip),
		Nodeport:         nodeport,
		Nsip:             net.ParseIP(jobNsIP(instance)),
		Nsipv6:           net.ParseIP(fmt.Sprintf("fc00::%x", instance+1)),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.0.1"),
			Address_v6: net.ParseIP("fdff::1"),
		}},
	}
}

func jobEntries(job string, n int, nodeip string, nodeport int) []TableEntry {
	entries := make([]TableEntry, 0, n)
	for i := range n {
		entries = append(entries, jobEntry(job, i, nodeip, nodeport))
	}
	return entries
}

func TestReplaceJobEntriesSwapsTheWholeSet(t *testing.T) {
	table := NewTableManager()
	if err := table.ReplaceJobEntries("job", jobEntries("job", 5, "10.0.0.1", 50103)); err != nil {
		t.Fatal(err)
	}

	entries, _ := table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))
	if len(entries) != 5 {
		t.Fatalf("indexed %d entries; want 5", len(entries))
	}

	// fewer instances on refresh must drop the extras, not merge them
	if err := table.ReplaceJobEntries("job", jobEntries("job", 2, "10.0.0.9", 50104)); err != nil {
		t.Fatal(err)
	}
	entries, _ = table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))
	if len(entries) != 2 {
		t.Fatalf("indexed %d entries after the refresh; want 2", len(entries))
	}
	for _, entry := range entries {
		if !entry.Nodeip.Equal(net.ParseIP("10.0.0.9")) || entry.Nodeport != 50104 {
			t.Errorf("stale route survived the refresh: %s:%d", entry.Nodeip, entry.Nodeport)
		}
	}
}

func TestReplaceJobEntriesRebuildsIndexesOnce(t *testing.T) {
	table := NewTableManager()

	_, before := table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))
	if err := table.ReplaceJobEntries("job", jobEntries("job", 50, "10.0.0.1", 50103)); err != nil {
		t.Fatal(err)
	}
	_, after := table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))

	if after-before != 1 {
		t.Errorf("replacing 50 entries rebuilt the indexes %d times; want 1", after-before)
	}
}

func TestReplaceJobEntriesRejectsInvalidInputAtomically(t *testing.T) {
	table := NewTableManager()
	if err := table.ReplaceJobEntries("job", jobEntries("job", 3, "10.0.0.1", 50103)); err != nil {
		t.Fatal(err)
	}
	_, generation := table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))

	bad := jobEntries("job", 3, "10.0.0.1", 50103)
	bad[2].Nodeip = nil
	if err := table.ReplaceJobEntries("job", bad); err == nil {
		t.Fatal("expected an error for an entry with no node IP")
	}

	entries, after := table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))
	if len(entries) != 3 {
		t.Errorf("table holds %d entries after a rejected replacement; want the original 3", len(entries))
	}
	if after != generation {
		t.Error("a rejected replacement still mutated the table")
	}
}

// an empty list means the job has no instances, which is not an invalid input
func TestReplaceJobEntriesEmptyListClearsJob(t *testing.T) {
	table := NewTableManager()
	if err := table.ReplaceJobEntries("job", jobEntries("job", 3, "10.0.0.1", 50103)); err != nil {
		t.Fatal(err)
	}
	if err := table.ReplaceJobEntries("job", nil); err != nil {
		t.Fatal(err)
	}
	if got := len(table.SearchByJobName("job")); got != 0 {
		t.Errorf("job still has %d entries", got)
	}
}

// a namespace IP claimed by a new entry must displace whoever held it, even across jobs
func TestReplaceJobEntriesDisplacesNamespaceIPs(t *testing.T) {
	table := NewTableManager()
	stale := jobEntry("oldjob", 0, "10.0.0.1", 50103)
	if err := table.Add(stale); err != nil {
		t.Fatal(err)
	}

	// instance 0 reuses the same namespace IP as `stale`
	if err := table.ReplaceJobEntries("newjob", []TableEntry{jobEntry("newjob", 0, "10.0.0.2", 50103)}); err != nil {
		t.Fatal(err)
	}

	entry, found := table.SearchByNsIP(netip.MustParseAddr(jobNsIP(0)))
	if !found {
		t.Fatal("namespace IP is no longer indexed")
	}
	if entry.JobName != "newjob" {
		t.Errorf("namespace IP still resolves to %q; want newjob", entry.JobName)
	}
	if len(table.SearchByJobName("oldjob")) != 0 {
		t.Error("the displaced entry is still in the table")
	}
}

func TestReplaceJobEntriesSharesActivity(t *testing.T) {
	table := NewTableManager()
	if err := table.ReplaceJobEntries("job", jobEntries("job", 3, "10.0.0.1", 50103)); err != nil {
		t.Fatal(err)
	}

	entries := table.SearchByJobName("job")
	if len(entries) != 3 {
		t.Fatalf("got %d entries; want 3", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].activity != entries[0].activity {
			t.Error("instances of one job must share a single activity tracker")
		}
	}
	if entries[0].activity == nil {
		t.Error("activity tracker was not attached")
	}
}

// the generation must move on any route change, or a cached route would never get revalidated
func TestGenerationTracksRouteChanges(t *testing.T) {
	table := NewTableManager()
	if err := table.ReplaceJobEntries("job", jobEntries("job", 1, "10.0.0.1", 50103)); err != nil {
		t.Fatal(err)
	}
	entries, _ := table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))

	nsip := netip.MustParseAddr(jobNsIP(0))
	if MatchRoute(nsip, netip.MustParseAddr("10.0.0.1"), 50103, entries) == nil {
		t.Fatal("the route should be valid before anything changes")
	}

	for name, replacement := range map[string][]TableEntry{
		"node IP change":   jobEntries("job", 1, "10.0.0.2", 50103),
		"node port change": jobEntries("job", 1, "10.0.0.1", 50104),
		"instance removal": {},
	} {
		t.Run(name, func(t *testing.T) {
			fresh := NewTableManager()
			if err := fresh.ReplaceJobEntries("job", jobEntries("job", 1, "10.0.0.1", 50103)); err != nil {
				t.Fatal(err)
			}
			_, before := fresh.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))
			if err := fresh.ReplaceJobEntries("job", replacement); err != nil {
				t.Fatal(err)
			}
			after, generationAfter := fresh.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))

			if generationAfter == before {
				t.Error("the generation did not move, so cached routes would never be rechecked")
			}
			if MatchRoute(nsip, netip.MustParseAddr("10.0.0.1"), 50103, after) != nil {
				t.Error("the old route is still considered valid")
			}
		})
	}
}

func TestTableConcurrentSearchAndReplace(t *testing.T) {
	table := NewTableManager()
	if err := table.ReplaceJobEntries("job", jobEntries("job", 20, "10.0.0.1", 50103)); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				// must never observe a table mid-replacement
				if entries, _ := table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1")); len(entries) != 20 {
					t.Errorf("read a partially replaced table: %d entries", len(entries))
					return
				}
				table.SearchByNsIP(netip.MustParseAddr(jobNsIP(0)))
			}
		}()
	}

	for i := range 200 {
		if err := table.ReplaceJobEntries("job", jobEntries("job", 20, fmt.Sprintf("10.0.0.%d", i%250+1), 50103)); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

func BenchmarkReplaceJobEntries(b *testing.B) {
	for _, n := range []int{100, 500} {
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			entries := jobEntries("job", n, "10.0.0.1", 50103)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				table := NewTableManager()
				if err := table.ReplaceJobEntries("job", entries); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// every Add rebuilds every index, so this is O(n^2) in the instance count
func BenchmarkAddOneByOne(b *testing.B) {
	for _, n := range []int{100, 500} {
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			entries := jobEntries("job", n, "10.0.0.1", 50103)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				table := NewTableManager()
				for _, entry := range entries {
					if err := table.Add(entry); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func BenchmarkRemoveByJobName(b *testing.B) {
	for _, n := range []int{100, 500} {
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			entries := jobEntries("job", n, "10.0.0.1", 50103)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				table := NewTableManager()
				if err := table.ReplaceJobEntries("job", entries); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if err := table.RemoveByJobName("job"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// only reached on a generation mismatch; steady state skips this scan entirely
func BenchmarkMatchRoute(b *testing.B) {
	for _, replicas := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("replicas=%d", replicas), func(b *testing.B) {
			table := NewTableManager()
			if err := table.ReplaceJobEntries("job", jobEntries("job", replicas, "10.0.0.1", 50103)); err != nil {
				b.Fatal(err)
			}
			entries, _ := table.SearchByServiceIP(netip.MustParseAddr("10.30.0.1"))
			// worst case: the matching replica is the last one scanned
			nsip := netip.MustParseAddr(jobNsIP(replicas - 1))
			node := netip.MustParseAddr("10.0.0.1")

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				MatchRoute(nsip, node, 50103, entries)
			}
		})
	}
}
