package TableEntryCache

import (
	"fmt"
	"net"
	"net/netip"
	"testing"
)

// n entries, each with a distinct NsIP and ServiceIP
func populatedTable(n int) *TableManager {
	table := NewTableManager()
	for i := range n {
		entry := TableEntry{
			Appname:          "app",
			Appns:            "ns",
			Servicename:      "svc",
			Servicenamespace: "svcns",
			Instancenumber:   i,
			Cluster:          0,
			Nodeip:           net.ParseIP(fmt.Sprintf("10.%d.%d.1", i/250, i%250)),
			Nodeport:         50103,
			Nsip:             net.ParseIP(fmt.Sprintf("10.19.%d.%d", i/250, i%250+1)),
			Nsipv6:           net.ParseIP(fmt.Sprintf("fc00::%x", i+1)),
			ServiceIP: []ServiceIP{{
				IpType:     RoundRobin,
				Address:    net.ParseIP(fmt.Sprintf("10.30.%d.%d", i/250, i%250)),
				Address_v6: net.ParseIP(fmt.Sprintf("fdff:2000::%x", i)),
			}},
		}
		if err := table.Add(entry); err != nil {
			panic(err)
		}
	}
	return &table
}

func BenchmarkSearchByServiceIP(b *testing.B) {
	for _, n := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			table := populatedTable(n)
			// last entry inserted, the worst case for a linear scan
			target := netip.MustParseAddr(fmt.Sprintf("10.30.%d.%d", (n-1)/250, (n-1)%250))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if res, _ := table.SearchByServiceIP(target); len(res) != 1 {
					b.Fatalf("expected 1 match, got %d", len(res))
				}
			}
		})
	}
}

func BenchmarkSearchByNsIP(b *testing.B) {
	for _, n := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			table := populatedTable(n)
			target := netip.MustParseAddr(fmt.Sprintf("10.19.%d.%d", (n-1)/250, (n-1)%250+1))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, found := table.SearchByNsIP(target); !found {
					b.Fatal("expected match")
				}
			}
		})
	}
}
