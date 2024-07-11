# NetManager
This component enables the communication between services distributed across multiple nodes.

The Network manager is divided in 4 main components: 

* Environment Manager: Creates the Host Bridge, is responsible for the creation and destruction of network namespaces, and for the maintenance of the Translation Table used by the other components. 
* ProxyTunnel: This is the communication channel. This component enables the service to service communication within the platform. In order to enable the communication the translation table must be kept up to date, otherwise this component asks the Environment manager for the "table query" resolution process. Refer to the official documentation for more details. 
* mDNS: (To be implemented) used for .local name resolution. Refer to the documentation for details.
* API: used to trigger a new deployment, the management operations on top of the already deployed services and to receive information about the services. 

# Structure

```

.
├── build/
│			Description:
│				Build and instalaltion scripts
├── config/
│			Description:
│				Configuration files used by the environment maanger and the proxyTunnel. 
├── env/
│			Description:
│				The environment manager implementation resides here. 
├── proxy/
│			Description:
│				This is where the ProxyTunnel implmentation belongs
├── mqtt/
│			Description:
│				Mqtt client implementation for cluster service manager routes resolution and subnetwork management.
├── cmd/
│			Description:
│				CLI commands
├── handlers/
│			Description:
│       dispatching methods for container or unikernel network management
├── server/
│			Description:
│       http REST server for incoming requests from NodeEngine
├── logger/
│			Description:
│       implementation of the NetManager logging framework
├── utils/
│			Description:
│       Just utility code 
└──  NetManager.go
			Description:
				Entry point to startup the NetworkManager

```

# Installation

- Navigate the `build` directory and use `./build.sh`
- Move the binary to current folder based on required architecture. E.g., `mv bin/amd64-NetManager NetManager` for amd64
- Then install it using `./install.sh` 

# Run NetManager

## 1) Prepare a config file

You can edit the default one placed in `/etc/netmanager/netcfg.json`
or you can create a custom one a pass the location at startup using the flag `--cfg="file location path"`

The netcfg file must contain the following fields

```

{
  "NodePublicAddress": "address",
  "NodePublicPort": "port",
  "ClusterUrl": "url",
  "ClusterMqttPort": "port"
}

```

## 2) Run the netmanager

The net manager must have root privileges. The host machine must have the *ip* command line tool installed.

Run the netmanager using:

`sudo NetManager`

## 3) supported startup flags

- `--cfg="file path"` allows you to set a custom location for a different net configuration file. 


## Development setup
The development setup can be used to test locally the tunneling mechanism without the use of the Cluster orchestrator. This setup requires 2 different machines namely Host1 and Host2.
* go 1.12+ required 
* run the install.sh to install the dependencies on each machine 

### todo: start the netmanager in debug mode 

## Subnetworks
With this default test configuration the Subnetwork hierarchy is:

###Container Network:
`10.16.0.0/12`

This is the network where all the IP addresses belongs

###Service IP subnetwork:
`10.32.0.0/16`

This is a special subnetwork where all the VirtualIP addresses belongs. An address belonging to this range must be
translated to an actual container address and pass trough the proxy. 

###Bridge Subnetwork:
`10.19.1.0/24`

Address where all the containers of this node belong. Each new container will have an address from this space.

###Prohibited port numbers
Right now a deployed service can't use the same port as the proxy tunnel
