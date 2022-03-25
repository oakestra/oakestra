package playground

import (
	"NetManager/env"
	"fmt"
	"net"
	"strconv"
)

func attachNetwork(appname string, pid int, instance int, mappings map[int]int, iip string, sip string) (string, error) {

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

	return addr.String(), nil
}

func AddRoute(entry env.TableEntry) {
	ENV.AddTableQueryEntry(entry)
	Entries = append(Entries, EntryToString(entry))
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
	return []string{entry.Appname, string(entry.Nsip), string(entry.ServiceIP[0].Address), string(entry.ServiceIP[1].Address), string(entry.Nodeip), strconv.Itoa(entry.Nodeport), strconv.Itoa(entry.Instancenumber)}
}
