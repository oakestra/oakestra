package main

import (
	"NetManager/env"
	"NetManager/proxy"
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

/*

#
# This source can be use to test the communication between up to 250 nodes
# 1- At boot the script asks for all the information regarding the overlay network
# 2- Provide all the necessary information that this node must know regarding the topology
# 2.1- ServiceIP, InstanceIP, hostIP, port, and nsIP of all the other services, plus the subnetwork of the current node
# 3- Use the deploy function to deploy a new network namespace with the given ServiceIP, InstanceIP and instance number
# 4- Log into the network namespace using the linux ip util and try the communication using the destination service IP

*/

type service struct {
	name     string
	rrip     string
	iip      string
	nsip     string
	instance int
}

func main() {
	//ask worker number
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("############################\n")
	fmt.Print("#### NetManager Startup ####\n")
	fmt.Print("############################\n")
	fmt.Print("Insert worker ID (number from 0 to 250):")
	workers, _ := reader.ReadString('\n')
	workers = strings.TrimSuffix(workers, "\n")
	workernum, err := strconv.Atoi(workers)
	if err != nil {
		log.Fatal(err)
	}

	//startup
	_, myenv := startup(workers)
	services := make([]service, 0)

	//deploy and register menu
	for true {
		fmt.Print("______________________________\n")
		fmt.Print("|       Select option:       |\n")
		fmt.Print("| 1 - Deploy service         |\n")
		fmt.Print("| 2 - Config list            |\n")
		fmt.Print("| 3 - Add service route      |\n")
		fmt.Print("|____________________________|\n")

		otpionstr, _ := reader.ReadString('\n')
		otpionstr = strings.TrimSuffix(otpionstr, "\n")
		option, err := strconv.Atoi(otpionstr)
		if err != nil {
			log.Fatal(err)
		}

		switch option {
		case 1:
			fmt.Print("service name:")
			servicename, _ := reader.ReadString('\n')
			servicename = strings.TrimSuffix(servicename, "\n")
			fmt.Print("instance ordinal number:")
			instancestr, _ := reader.ReadString('\n')
			instancestr = strings.TrimSuffix(instancestr, "\n")
			instance, err := strconv.Atoi(instancestr)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Print("RoundRobin IP (172.30.x.y):")
			rrip, _ := reader.ReadString('\n')
			rrip = strings.TrimSuffix(rrip, "\n")
			fmt.Print("Instance IP (172.30.x.y):")
			iip, _ := reader.ReadString('\n')
			iip = strings.TrimSuffix(iip, "\n")

			nsip := deploy(myenv, servicename, workernum, len(services), instance, rrip, iip)

			services = append(services, service{
				name:     servicename,
				rrip:     rrip,
				iip:      iip,
				nsip:     nsip,
				instance: instance,
			})

		case 2:
			fmt.Print("______________________________\n")
			for i, s := range services {
				fmt.Println(s)
				if i != len(services)-1 {
					fmt.Print("____\n")
				}
			}
			fmt.Print("______________________________\n")

		case 3:
			fmt.Print("service name:")
			servicename, _ := reader.ReadString('\n')
			servicename = strings.TrimSuffix(servicename, "\n")
			fmt.Print("instance ordinal number:")
			instancestr, _ := reader.ReadString('\n')
			instancestr = strings.TrimSuffix(instancestr, "\n")
			instance, err := strconv.Atoi(instancestr)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Print("RoundRobin IP (172.30.x.y):")
			rrip, _ := reader.ReadString('\n')
			rrip = strings.TrimSuffix(rrip, "\n")
			fmt.Print("Instance IP (172.30.x.y):")
			iip, _ := reader.ReadString('\n')
			iip = strings.TrimSuffix(iip, "\n")
			fmt.Print("nsIP (172.z.x.y):")
			nsip, _ := reader.ReadString('\n')
			nsip = strings.TrimSuffix(nsip, "\n")
			fmt.Print("node IP (x.y.z.a):")
			nodeip, _ := reader.ReadString('\n')
			nodeip = strings.TrimSuffix(nodeip, "\n")

			registerService(myenv, servicename, instance, nodeip, nsip, rrip, iip)

			fmt.Print("## Route registered successfully! ## \n")

		}

	}

}

func startup(workernum string) (*proxy.GoProxyTunnel, *env.Environment) {
	tunconfig := proxy.Configuration{
		HostTUNDeviceName:   "goProxyTun",
		ProxySubnetwork:     "172.30.0.0",
		ProxySubnetworkMask: "255.255.0.0",
		TunNetIP:            "172.19." + workernum + ".254",
		TunnelPort:          50000,
		Mtusize:             "1450",
	}

	config := env.Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               "172.19." + workernum + ".1",
		HostBridgeMask:             "/24",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "",
		Mtusize:                    "1450",
	}

	fmt.Println("Creating new proxyTUN...")

	myproxy := proxy.NewCustom(tunconfig)
	myproxy.Listen()

	time.Sleep(4 * time.Second)

	fmt.Println("Creating new EnvironmentManager...")

	//creating twice just to cleanup eventual former trash
	myenv := env.NewCustom(myproxy.GetName(), config)
	myenv.Destroy()
	myenv = env.NewCustom(myproxy.GetName(), config)

	myproxy.SetEnvironment(&myenv)

	return &myproxy, &myenv
}

func deploy(myenv *env.Environment,
	servicename string,
	workernum int,
	servicenum int,
	instance int,
	rrip string,
	iip string) string {

	nsip := "172.19." + strconv.Itoa(workernum) + "." + strconv.Itoa(servicenum+3)

	fmt.Println("Generating namesapce for ", servicename, " with nsIP ", nsip)
	ip := net.ParseIP(nsip)
	_, err := myenv.CreateNetworkNamespace(servicename, ip)
	if err != nil {
		fmt.Println(err)
	}

	currentip, _ := env.GetLocalIPandIface()
	registerService(myenv, servicename, instance, currentip, nsip, rrip, iip)

	return nsip
}

func undeploy() {
	//TODO
}

func registerService(
	myenv *env.Environment,
	servicename string,
	instance int,
	nodeip string,
	nsip string,
	rrip string,
	iip string) {

	myenv.AddTableQueryEntry(env.TableEntry{
		Appname:          "app1",
		Appns:            "default",
		Servicename:      servicename,
		Servicenamespace: "default",
		Instancenumber:   instance,
		Cluster:          0,
		Nodeip:           net.ParseIP(nodeip),
		Nodeport:         50000,
		Nsip:             net.ParseIP(nsip),
		ServiceIP: []env.ServiceIP{{
			IpType:  env.RoundRobin,
			Address: net.ParseIP(rrip),
		}, {
			IpType:  env.InstanceNumber,
			Address: net.ParseIP(iip),
		}},
	})
}

func unsergisterService(myenv *env.Environment) {
	//TODO
}
