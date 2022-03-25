package playground

import (
	"NetManager/env"
	"fmt"
	"net"
)

func attachNetwork(appname string, pid int, instance int, mappings map[int]int, iip string, sip string) (string, error) {

	//attach network to the container
	addr, err := ENV.AttachNetworkToContainer(pid, appname, mappings)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return "", err
	}

	//update internal table entry
	ENV.AddTableQueryEntry(env.TableEntry{
		JobName:          appname,
		Appname:          appname,
		Appns:            "default",
		Servicename:      "test",
		Servicenamespace: "default",
		Instancenumber:   instance,
		Cluster:          0,
		Nodeip:           net.IP(PUBLIC_ADDRESS),
		Nodeport:         PUBLIC_PORT,
		Nsip:             addr,
		ServiceIP: []env.ServiceIP{
			env.ServiceIP{
				IpType:  env.InstanceNumber,
				Address: net.IP(iip),
			},
			env.ServiceIP{
				IpType:  env.RoundRobin,
				Address: net.IP(sip),
			},
		},
	})

	return addr.String(), nil
}
