package playground

import (
	"NetManager/TableEntryCache"
	"NetManager/env"
	"fmt"
	"net"
	"strconv"
)

func attachNetwork(appname string, pid int, instance int, mappings string, iip string, sip string) (string, error) {

	//attach network to the container
	// TODO IPv6 playground implementation: _ = addrv6
	addr, _, err := env.GetContainerNetDeployment().DeployNetwork(pid, appname, 0, mappings)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return "", err
	}

	//update internal table entry
	AddRoute(TableEntryCache.TableEntry{
		JobName:          appname,
		Appname:          appname,
		Appns:            "default",
		Servicename:      "test",
		Servicenamespace: "default",
		Instancenumber:   instance,
		Cluster:          0,
		Nodeip:           net.ParseIP(PUBLIC_ADDRESS),
		Nodeport:         PUBLIC_PORT,
		Nsip:             addr,
		ServiceIP: []TableEntryCache.ServiceIP{
			TableEntryCache.ServiceIP{
				IpType:  TableEntryCache.InstanceNumber,
				Address: net.ParseIP(iip),
			},
			TableEntryCache.ServiceIP{
				IpType:  TableEntryCache.RoundRobin,
				Address: net.ParseIP(sip),
			},
		},
	})

	return fmt.Sprintf("%s", addr.String()), nil
}

func AddRoute(entry TableEntryCache.TableEntry) {
	if !entryExist(entry) {
		ENV.AddTableQueryEntry(entry)
		Entries = append(Entries, EntryToString(entry))
	}
}

func StringToEntry(entry []string) TableEntryCache.TableEntry {
	port, _ := strconv.Atoi(entry[5])
	instance, _ := strconv.Atoi(entry[6])
	return TableEntryCache.TableEntry{
		JobName:          entry[0],
		Appname:          entry[0],
		Appns:            "default",
		Servicename:      "test",
		Servicenamespace: "default",
		Instancenumber:   instance,
		Cluster:          0,
		Nodeip:           net.ParseIP(entry[4]),
		Nodeport:         port,
		Nsip:             net.ParseIP(entry[1]),
		ServiceIP: []TableEntryCache.ServiceIP{
			{
				IpType:  TableEntryCache.InstanceNumber,
				Address: net.ParseIP(entry[2]),
			},
			{
				IpType:  TableEntryCache.RoundRobin,
				Address: net.ParseIP(entry[3]),
			},
		},
	}
}

func EntryToString(entry TableEntryCache.TableEntry) []string {
	return []string{entry.Appname, fmt.Sprintf("%s", entry.Nsip.String()), fmt.Sprintf("%s", entry.ServiceIP[0].Address.String()), fmt.Sprintf("%s", entry.ServiceIP[1].Address.String()), fmt.Sprintf("%s", entry.Nodeip.String()), strconv.Itoa(entry.Nodeport), strconv.Itoa(entry.Instancenumber)}
}

func entryExist(toAdd TableEntryCache.TableEntry) bool {
	for _, entry := range Entries {
		if entry[1] == fmt.Sprintf("%s", toAdd.Nsip.String()) {
			return true
		}
	}
	return false
}
