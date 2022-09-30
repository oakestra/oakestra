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
│				Binary executable compiled files and build scripts
├── config/
│			Description:
│				Configuration files used by the environment maanger and the proxyTunnel. These configuration files are used only 
│               for testing purpose to create a local environment without the need of plugging the compennt to the local orchestrator. 
├── env/
│			Description:
│				The environment manager implementation resides here. 
├── proxy/
│			Description:
│				This is where the ProxyTunnel implmentation belongs
├── testEnvironment/
│			Description:
│				Executable files that can be used to test the Netowrk Manager locally. 
├── mqtt/
│			Description:
│				Mqtt interface witht he cluster service manager
├── install.sh
│			Description:
│				installation script 
└──  NetManager.go
			Description:
				Entry point to startup the NetworkManager

```

# Installation

- download the latest release from [here](https://github.com/edgeIO/edgeionet/releases)
- run `./install.sh <architecture>` specifying amd64 or arm-7

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


## Deployment
Note, most of the following must still be implemented

### With binary files

Execute the binary files directly specifying the Cluster address. This will trigger the registration procedure. 
`sudo ./bin/Networkmanager -cluster <ip-address>`

### With go commandline

* go 1.12+ required
* run the setup.sh to install the dependencies on each machine

Execute the Network manager with 
`sudo go run NetManager.go -cluster <ip-address>`
