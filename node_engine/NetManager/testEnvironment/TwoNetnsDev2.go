package main

import (
	"../env"
	"../proxy"
	"fmt"
	"log"
	"net"
)

func main() {
	fmt.Println("Create the goProxy tun device")
	myproxy := proxy.New()
	myproxy.Listen()
	errch := myproxy.GetErrCh()
	stopch := myproxy.GetStopCh()
	finishch := myproxy.GetFinishCh()

	config := env.Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               "172.19.2.1",
		HostBridgeMask:             "/24",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "wlan0",
	}
	myenv := env.NewCustom(myproxy.GetName(), config)

	//Cleanup
	myenv.Destroy()
	myenv = env.NewCustom(myproxy.GetName(), config)
	fmt.Println("Initial env: \n ", myenv)
	fmt.Println("Creating service 1 with ip 172.19.2.12 and namespace myapp1")
	ip1 := net.ParseIP("172.19.2.12")
	_, err := myenv.CreateNetworkNamespace("myapp1", ip1)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Creating service 2 with ip 172.19.2.15 and namespace myapp2")
	ip2 := net.ParseIP("172.19.2.15")
	_, err = myenv.CreateNetworkNamespace("myapp2", ip2)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Final env: \n ", myenv)

	//listen tun device
	cherror := <-errch
	<-finishch
	stopch <- true
	log.Fatal(cherror)
}
