package playground

import (
	"NetManager/env"
	"fmt"
	"net"
	"strconv"
)

func attachNetwork(appname string, pid int, instance int, mappings string, iip string, sip string) (string, error) {

	//attach network to the container
	addr, err := ENV.AttachNetworkToContainer(pid, appname, mappings)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return "", err
	}

	//update internal table entry
	AddRoute(env.TableEntry{
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
		ServiceIP: []env.ServiceIP{
			env.ServiceIP{
				IpType:  env.InstanceNumber,
				Address: net.ParseIP(iip),
			},
			env.ServiceIP{
				IpType:  env.RoundRobin,
				Address: net.ParseIP(sip),
			},
		},
	})

	return fmt.Sprintf("%s", addr.String()), nil
}

func AddRoute(entry env.TableEntry) {
	if !entryExist(entry) {
		ENV.AddTableQueryEntry(entry)
		Entries = append(Entries, EntryToString(entry))
	}
}

func StringToEntry(entry []string) env.TableEntry {
	port, _ := strconv.Atoi(entry[5])
	instance, _ := strconv.Atoi(entry[6])
	return env.TableEntry{
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
		ServiceIP: []env.ServiceIP{
			{
				IpType:  env.InstanceNumber,
				Address: net.ParseIP(entry[2]),
			},
			{
				IpType:  env.RoundRobin,
				Address: net.ParseIP(entry[3]),
			},
		},
	}
}

func EntryToString(entry env.TableEntry) []string {
	return []string{entry.Appname, fmt.Sprintf("%s", entry.Nsip.String()), fmt.Sprintf("%s", entry.ServiceIP[0].Address.String()), fmt.Sprintf("%s", entry.ServiceIP[1].Address.String()), fmt.Sprintf("%s", entry.Nodeip.String()), strconv.Itoa(entry.Nodeport), strconv.Itoa(entry.Instancenumber)}
}

func entryExist(toAdd env.TableEntry) bool {
	for _, entry := range Entries {
		if entry[1] == fmt.Sprintf("%s", toAdd.Nsip.String()) {
			return true
		}
	}
	return false
}
