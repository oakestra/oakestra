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

func main() {
	//setup
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Current Dev IP address for demonstrative purpose: \n")
	host1, _ := env.GetLocalIPandIface()
	host1 = strings.TrimSuffix(host1, "\n")
	fmt.Println("Current Host ip set to: ", host1)
	fmt.Print("Input Client machine IP address for demonstrative purpose: \n")
	host2, _ := reader.ReadString('\n')
	//host2 := "192.168.42.165"
	host2 = strings.TrimSuffix(host2, "\n")
	fmt.Println("Dev2 Host ip set to: ", host2)
	fmt.Print("Name of the docker container currently deployed that must be plugged into NetManager: \n")
	containername, _ := reader.ReadString('\n')
	//containername := "mynginx1"
	containername = strings.TrimSuffix(containername, "\n")
	fmt.Println("Docker container used: ", containername)
	fmt.Print("Instance number: \n")
	instance, _ := reader.ReadString('\n')
	instance = strings.TrimSuffix(instance, "\n")
	fmt.Println("Instance: ", instance)
	intinstance, _ := strconv.Atoi(instance)
	fmt.Print("MTU size: \n")
	mtusize, _ := reader.ReadString('\n')
	mtusize = strings.TrimSuffix(mtusize, "\n")

	//create the tunnel
	fmt.Println("Create the goProxy tun device")

	tunconfig := proxy.Configuration{
		HostTUNDeviceName:   "goProxyTun",
		ProxySubnetwork:     "172.30.0.0",
		ProxySubnetworkMask: "255.255.0.0",
		TunNetIP:            "172.19." + instance + ".254",
		TunnelPort:          50011,
		Mtusize:             mtusize,
	}

	myproxy := proxy.NewCustom(tunconfig)
	myproxy.Listen()
	errch := myproxy.GetErrCh()
	_ = myproxy.GetStopCh()
	finishch := myproxy.GetFinishCh()

	//create the env and the namespaces
	config := env.Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               "172.19." + instance + ".1",
		HostBridgeMask:             "/24",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "",
		Mtusize:                    mtusize,
	}

	time.Sleep(4 * time.Second)

	//Cleanup and create a new environment
	myenv := env.NewCustom(myproxy.GetName(), config)
	myenv.Destroy()
	myenv = env.NewCustom(myproxy.GetName(), config)
	fmt.Println("Binding Docker container ", containername)
	ip2, err := myenv.AttachDockerContainer(containername)
	fmt.Println("Deployed container with ip ", ip2.String())

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Final env: \n ", myenv)

	myproxy.SetEnvironment(&myenv)

	//Setup self
	myenv.AddTableQueryEntry(env.TableEntry{
		Appname:          "nettest",
		Appns:            "default",
		Servicename:      "server",
		Servicenamespace: "default",
		Instancenumber:   intinstance,
		Cluster:          0,
		Nodeip:           net.ParseIP(host1),
		Nodeport:         50011,
		Nsip:             ip2,
		ServiceIP: []env.ServiceIP{{
			IpType:  env.RoundRobin,
			Address: net.ParseIP("172.30.25.25"),
		}, {
			IpType:  env.InstanceNumber,
			Address: net.ParseIP("172.30.0." + instance),
		}},
	})
	//Setup client
	myenv.AddTableQueryEntry(env.TableEntry{
		Appname:          "nettest",
		Appns:            "default",
		Servicename:      "client",
		Servicenamespace: "default",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP(host2),
		Nodeport:         50011,
		Nsip:             net.ParseIP("172.20.1.2"),
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
