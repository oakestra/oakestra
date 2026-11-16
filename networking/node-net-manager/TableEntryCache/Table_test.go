package TableEntryCache

import (
	"fmt"
	"net"
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
