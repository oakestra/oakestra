package TableEntryCache

import (
	"fmt"
	"net"
	"net/netip"
	"testing"
)

func TestTableInsertSuccessfull(t *testing.T) {
	table := NewTableManager()
	entry := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}

	err := table.Add(entry)
	if err != nil {
		t.Error("Error during insertion")
	}

	if table.translationTable[0].Appname != "a1" {
		t.Error("Invalid first element")
	}
}

func TestTableInsertError(t *testing.T) {
	table := NewTableManager()
	entry := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           nil,
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}

	err := table.Add(entry)
	if err == nil {
		t.Error("Insertion should have thrown an error")
	}
}

func TestTableDeleteOne(t *testing.T) {
	table := NewTableManager()
	entry := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}

	_ = table.Add(entry)

	err := table.RemoveByNsip(net.ParseIP("10.18.0.1"))
	if err != nil {
		t.Error("Error during deletion")
	}

	if len(table.translationTable) > 0 {
		t.Error("Table size should be zero")
	}
}

func TestTableDeleteOne_v6(t *testing.T) {
	table := NewTableManager()
	entry := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}

	_ = table.Add(entry)

	err := table.RemoveByNsip(net.ParseIP("fc00::1"))
	if err != nil {
		t.Error("Error during deletion")
	}

	if len(table.translationTable) > 0 {
		t.Error("Table size should be zero")
	}
}

func TestTableDeleteMany(t *testing.T) {
	table := NewTableManager()
	entry1 := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}
	entry2 := TableEntry{
		Appname:          "a2",
		Appns:            "a2",
		Servicename:      "a3",
		Servicenamespace: "a3",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.21.1"),
		Nsipv6:           net.ParseIP("fc00::211"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}

	_ = table.Add(entry1)
	_ = table.Add(entry2)

	err := table.RemoveByNsip(net.ParseIP("10.18.21.1"))
	if err != nil {
		t.Error("Error during deletion")
	}

	if len(table.translationTable) > 1 {
		t.Error("Table size should be 1")
	}

	if table.translationTable[0].Appname != "a1" {
		t.Error("Removed the wrong entry")
	}
}

func TestTableDeleteMany_v6(t *testing.T) {
	table := NewTableManager()
	entry1 := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}
	entry2 := TableEntry{
		Appname:          "a2",
		Appns:            "a2",
		Servicename:      "a3",
		Servicenamespace: "a3",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.21.1"),
		Nsipv6:           net.ParseIP("fc00::211"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}

	_ = table.Add(entry1)
	_ = table.Add(entry2)

	err := table.RemoveByNsip(net.ParseIP("fc00::211"))
	if err != nil {
		t.Error("Error during deletion")
	}

	if len(table.translationTable) > 1 {
		t.Error("Table size should be 1")
	}

	if table.translationTable[0].Appname != "a1" {
		t.Error("Removed the wrong entry")
	}
}

func TestTableDeleteManyInstances_1(t *testing.T) {
	table := NewTableManager()
	entry1 := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		JobName:          "a1.a1.a2.a2",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000::1"),
		}},
	}
	entry2 := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		JobName:          "a1.a1.a2.a2",
		Instancenumber:   1,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.2"),
		Nsipv6:           net.ParseIP("fc00::2"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.2"),
			Address_v6: net.ParseIP("fdff:2000::2"),
		}},
	}

	entry3 := TableEntry{
		Appname:          "a3",
		Appns:            "a3",
		Servicename:      "a2",
		Servicenamespace: "a2",
		JobName:          "a3.a3.a2.a2",
		Instancenumber:   1,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.3"),
		Nsipv6:           net.ParseIP("fc00::3"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.3"),
			Address_v6: net.ParseIP("fdff:2000::3"),
		}},
	}

	_ = table.Add(entry1)
	_ = table.Add(entry2)
	_ = table.Add(entry3)

	err := table.RemoveByJobName("a1.a1.a2.a2")
	if err != nil {
		t.Errorf("Error during deletion: %v", err)
	}

	if len(table.translationTable) > 1 {
		t.Error("Table size should be 1")
	}

	if len(table.SearchByJobName("a1.a1.a2.a2")) > 0 {
		t.Errorf("a1 should not be there: %v", table.SearchByJobName("a1.a1.a2.a2"))
	}
}

func TestTableDeleteManyInstances_2(t *testing.T) {
	table := NewTableManager()
	entry1 := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		JobName:          "a1.a1.a2.a2",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.1"),
			Address_v6: net.ParseIP("fdff:2000:1"),
		}},
	}
	entry2 := TableEntry{
		Appname:          "a1",
		Appns:            "a1",
		Servicename:      "a2",
		Servicenamespace: "a2",
		JobName:          "a1.a1.a2.a2",
		Instancenumber:   1,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("10.18.0.2"),
		Nsipv6:           net.ParseIP("fc00::2"),
		ServiceIP: []ServiceIP{{
			IpType:     RoundRobin,
			Address:    net.ParseIP("10.30.1.2"),
			Address_v6: net.ParseIP("fdff:2000:2"),
		}},
	}

	_ = table.Add(entry1)
	_ = table.Add(entry2)

	err := table.RemoveByJobName("a1.a1.a2.a2")
	if err != nil {
		t.Errorf("Error during deletion: %v", err)
	}

	if len(table.translationTable) != 0 {
		fmt.Printf("%v", table.translationTable)
		t.Error(fmt.Sprintf("Table size should be 0, instead is %d", len(table.translationTable)))
	}

	if len(table.SearchByJobName("a1.a1.a2.a2")) > 0 {
		t.Errorf("a1 should not be there: %v", table.SearchByJobName("a1.a1.a2.a2"))
	}
}

// sharedSubnetEntry builds an entry for the shared-subnet tests: every node
// allocates namespace IPs from the same fixed subnet, so instances on different
// nodes routinely carry the same namespace IP.
func sharedSubnetEntry(job, service, nodeip, nsip, instanceIP string) TableEntry {
	return TableEntry{
		JobName: job, Appname: "app", Appns: "ns", Servicename: service, Servicenamespace: "ns",
		Nsip: net.ParseIP(nsip), Nsipv6: net.ParseIP("fc00::2"),
		Nodeip: net.ParseIP(nodeip), Nodeport: 50103,
		ServiceIP: []ServiceIP{
			{IpType: RoundRobin, Address: net.ParseIP("10.30.30.10"), Address_v6: net.ParseIP("fdff:2000::1")},
			{IpType: InstanceNumber, Address: net.ParseIP(instanceIP), Address_v6: net.ParseIP("fdff:2000::2")},
		},
	}
}

// TestReplaceJobEntriesSwapsAtomically covers the route refresh: rows for one job are
// swapped wholesale and other jobs are untouched.
func TestReplaceJobEntriesSwapsAtomically(t *testing.T) {
	table := NewTableManager()
	rr := netip.MustParseAddr("10.30.30.10")
	if err := table.Add(sharedSubnetEntry("app.ns.srv1.ns", "srv1", "10.99.0.11", "10.18.0.2", "10.30.1.1")); err != nil {
		t.Fatalf("seed srv1: %v", err)
	}
	if err := table.Add(sharedSubnetEntry("app.ns.srv2.ns", "srv2", "10.99.0.11", "10.18.0.3", "10.30.1.2")); err != nil {
		t.Fatalf("seed srv2: %v", err)
	}

	// srv1 moved to another node: one row in, one row out, srv2 left alone.
	moved := sharedSubnetEntry("app.ns.srv1.ns", "srv1", "10.99.0.12", "10.18.0.9", "10.30.2.1")
	if err := table.ReplaceJobEntries("app.ns.srv1.ns", []TableEntry{moved}); err != nil {
		t.Fatal(err)
	}

	behind, _ := table.SearchByServiceIP(rr)
	if len(behind) != 2 {
		t.Fatalf("expected 2 backends behind the ServiceIP, got %d: %+v", len(behind), behind)
	}
	for _, entry := range behind {
		if entry.JobName == "app.ns.srv1.ns" && !entry.Nodeip.Equal(net.ParseIP("10.99.0.12")) {
			t.Errorf("srv1 still points at the old node: %v", entry.Nodeip)
		}
		if entry.JobName == "app.ns.srv2.ns" && !entry.Nsip.Equal(net.ParseIP("10.18.0.3")) {
			t.Errorf("srv2 was disturbed by a replace of srv1: %+v", entry)
		}
	}

	// Replacing with nothing removes the job entirely (the undeploy case).
	if err := table.ReplaceJobEntries("app.ns.srv1.ns", nil); err != nil {
		t.Fatal(err)
	}
	if behind, _ := table.SearchByServiceIP(rr); len(behind) != 1 {
		t.Fatalf("expected 1 backend after removing srv1, got %d", len(behind))
	}
}

// TestNsIPLookupIsNotShadowedByARemoteNode: the namespace-IP lookups translate a
// packet's source, which is always a local instance. Picking a remote entry with the
// same namespace IP stamps that node's instance IP on our outgoing packets, and the
// replies are routed to that node instead of back here.
func TestNsIPLookupIsNotShadowedByARemoteNode(t *testing.T) {
	shared := netip.MustParseAddr("10.18.0.2")
	remote1 := sharedSubnetEntry("app.ns.srv1.ns", "srv1", "10.99.0.11", "10.18.0.2", "10.30.115.1")
	remote2 := sharedSubnetEntry("app.ns.srv2.ns", "srv2", "10.99.0.12", "10.18.0.2", "10.30.193.1")
	local := sharedSubnetEntry("app.ns.cli1.ns", "cli1", "10.99.0.13", "10.18.0.2", "10.30.232.2")

	for name, order := range map[string][]TableEntry{
		"local last":  {remote1, remote2, local},
		"local first": {local, remote1, remote2},
	} {
		t.Run(name, func(t *testing.T) {
			table := NewTableManager()
			table.SetLocalNode(net.ParseIP("10.99.0.13"))
			for _, entry := range order {
				if err := table.Add(entry); err != nil {
					t.Fatalf("seeding %s: %v", entry.JobName, err)
				}
			}
			if found, ok := table.SearchByNsIP(shared); !ok || found.JobName != "app.ns.cli1.ns" {
				t.Errorf("SearchByNsIP picked %s on %v; it must pick the local instance", found.JobName, found.Nodeip)
			}
			if addr, ok := table.SearchInstanceIPByNsIP(shared, 4); !ok || addr != netip.MustParseAddr("10.30.232.2") {
				t.Errorf("source instance IP = %v; want the local instance's 10.30.232.2", addr)
			}
		})
	}

	// Without a local node the first entry wins, as before.
	table := NewTableManager()
	for _, entry := range []TableEntry{remote1, remote2, local} {
		if err := table.Add(entry); err != nil {
			t.Fatal(err)
		}
	}
	if found, _ := table.SearchByNsIP(shared); found.JobName != "app.ns.srv1.ns" {
		t.Errorf("without a local node, lookup changed behaviour: got %s", found.JobName)
	}
}

// TestNamespaceIPCollisionsStayPerNode: replacing or removing one node's instance must
// not evict another node's instance that happens to share its namespace IP.
func TestNamespaceIPCollisionsStayPerNode(t *testing.T) {
	table := NewTableManager()
	table.SetLocalNode(net.ParseIP("10.99.0.13"))
	rr := netip.MustParseAddr("10.30.30.10")
	for _, entry := range []TableEntry{
		sharedSubnetEntry("app.ns.srv1.ns", "srv1", "10.99.0.11", "10.18.0.2", "10.30.1.1"),
		sharedSubnetEntry("app.ns.srv2.ns", "srv2", "10.99.0.12", "10.18.0.2", "10.30.2.1"),
		sharedSubnetEntry("app.ns.cli1.ns", "cli1", "10.99.0.13", "10.18.0.2", "10.30.3.1"),
	} {
		if err := table.Add(entry); err != nil {
			t.Fatal(err)
		}
	}

	// A refresh of srv1 must leave srv2 and the local cli1 alone.
	refreshed := sharedSubnetEntry("app.ns.srv1.ns", "srv1", "10.99.0.11", "10.18.0.2", "10.30.1.1")
	if err := table.ReplaceJobEntries("app.ns.srv1.ns", []TableEntry{refreshed}); err != nil {
		t.Fatal(err)
	}
	if behind, _ := table.SearchByServiceIP(rr); len(behind) != 3 {
		t.Fatalf("refresh evicted another node's instance: %+v", behind)
	}

	// Tearing down the local instance removes it, not a remote one listed earlier.
	if err := table.RemoveByNsip(net.ParseIP("10.18.0.2")); err != nil {
		t.Fatal(err)
	}
	behind, _ := table.SearchByServiceIP(rr)
	if len(behind) != 2 {
		t.Fatalf("expected the two remote instances to remain, got %+v", behind)
	}
	for _, entry := range behind {
		if entry.JobName == "app.ns.cli1.ns" {
			t.Errorf("local instance survived its teardown; a remote one was removed instead")
		}
	}
}
