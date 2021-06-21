package main

import (
	"NetManager/env"
	"NetManager/proxy"
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var Hosts = []string{
	"192.168.42.189", //vm19
	"192.168.42.185", //vm13
	"192.168.42.174", //vm14
	"192.168.42.175", //vm15
	"192.168.42.184", //vm16
	"192.168.42.151", //vm01
	"192.168.42.122", //vm02
	"192.168.42.141", //vm03
	"192.168.42.168", //vm10
}

func main() {
	//create the tunnel
	fmt.Println("Create the goProxy tun device")

	tunconfig := proxy.Configuration{
		HostTUNDeviceName:   "goProxyTun",
		ProxySubnetwork:     "172.30.0.0",
		ProxySubnetworkMask: "255.255.0.0",
		TunNetIP:            "172.20.1.254",
		TunnelPort:          50011,
	}

	myproxy := proxy.NewCustom(tunconfig)
	myproxy.Listen()
	errch := myproxy.GetErrCh()
	_ = myproxy.GetStopCh()
	finishch := myproxy.GetFinishCh()

	//create the env and the namespaces
	config := env.Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               "172.20.1.1",
		HostBridgeMask:             "/24",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "",
	}

	time.Sleep(4 * time.Second)
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Current Dev IP address for demonstrative purpose: \n")
	host1, _ := reader.ReadString('\n')
	host1 = strings.TrimSuffix(host1, "\n")
	fmt.Println("Current Host ip set to: ", host1)
	fmt.Print("Name of the docker container currently deployed that must be plugged into NetManager: \n")
	containername, _ := reader.ReadString('\n')
	containername = strings.TrimSuffix(containername, "\n")
	fmt.Println("Docker container used: ", containername)

	//Cleanup and create a new environment
	myenv := env.NewCustom(myproxy.GetName(), config)
	myenv.Destroy()
	myenv = env.NewCustom(myproxy.GetName(), config)
	fmt.Println("Initial env: \n ", myenv)

	fmt.Println("Binding Docker container ", containername)
	ip2, err := myenv.AttachDockerContainer(containername)
	fmt.Println("Deployed container with ip ", ip2.String())

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Final env: \n ", myenv)

	myproxy.SetEnvironment(&myenv)

	// Setup external services

	for i, host := range Hosts {
		myenv.AddTableQueryEntry(env.TableEntry{
			Appname:          "nettest",
			Appns:            "default",
			Servicename:      "server",
			Servicenamespace: "default",
			Instancenumber:   i,
			Cluster:          0,
			Nodeip:           net.ParseIP(host),
			Nodeport:         50011,
			Nsip:             net.ParseIP("172.19." + strconv.Itoa(i) + ".2"),
			ServiceIP: []env.ServiceIP{{
				IpType:  env.RoundRobin,
				Address: net.ParseIP("172.30.25.25"),
			}, {
				IpType:  env.InstanceNumber,
				Address: net.ParseIP("172.30.0." + strconv.Itoa(i)),
			}},
		})
	}

	//Setup self
	myenv.AddTableQueryEntry(env.TableEntry{
		Appname:          "nettest",
		Appns:            "default",
		Servicename:      "client",
		Servicenamespace: "default",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP(host1),
		Nodeport:         50011,
		Nsip:             ip2,
		ServiceIP: []env.ServiceIP{{
			IpType:  env.RoundRobin,
			Address: net.ParseIP("172.30.20.20"),
		}, {
			IpType:  env.InstanceNumber,
			Address: net.ParseIP("172.30.20.21"),
		}},
	})

	//listen tun device
	for {
		select {
		case _ = <-finishch:
			return
		case cherror := <-errch:
			print(cherror)
		}
	}
}
