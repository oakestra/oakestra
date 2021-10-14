package env

import (
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
		Nodeip:           net.ParseIP("172.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("172.18.0.1"),
		ServiceIP: []ServiceIP{{
			IpType:  RoundRobin,
			Address: net.ParseIP("172.30.1.1"),
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
		Nsip:             net.ParseIP("172.18.0.1"),
		ServiceIP: []ServiceIP{{
			IpType:  RoundRobin,
			Address: net.ParseIP("172.30.1.1"),
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
		Nodeip:           net.ParseIP("172.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("172.18.0.1"),
		ServiceIP: []ServiceIP{{
			IpType:  RoundRobin,
			Address: net.ParseIP("172.30.1.1"),
		}},
	}

	_ = table.Add(entry)

	err := table.RemoveByNsip(net.ParseIP("172.18.0.1"))
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
		Nodeip:           net.ParseIP("172.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("172.18.0.1"),
		ServiceIP: []ServiceIP{{
			IpType:  RoundRobin,
			Address: net.ParseIP("172.30.1.1"),
		}},
	}
	entry2 := TableEntry{
		Appname:          "a2",
		Appns:            "a2",
		Servicename:      "a3",
		Servicenamespace: "a3",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("172.30.0.1"),
		Nodeport:         1003,
		Nsip:             net.ParseIP("172.18.21"),
		ServiceIP: []ServiceIP{{
			IpType:  RoundRobin,
			Address: net.ParseIP("172.30.1.1"),
		}},
	}

	_ = table.Add(entry1)
	_ = table.Add(entry2)

	err := table.RemoveByNsip(net.ParseIP("172.18.21"))
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
