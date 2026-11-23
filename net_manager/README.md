# NetManager 🕸️
This component enables the communication between services distributed across multiple nodes and clusters.
It is the worker side of the Oakestra network: the daemon installed next to the NodeEngine on every worker node.
It captures the services traffic and creates the semantic overlay abstraction, where an IP address not only represents
the final destination of a packet but also enforces a balancing policy. See
[Semantic Addressing](https://www.oakestra.io/docs/networking/semantic-addressing) for details.

The control plane counterparts live with the orchestrators:

- [Root Service Manager](../root_orchestrator/service-manager/README.md) (`root_orchestrator/service-manager/`)
- [Cluster Service Manager](../cluster_orchestrator/service-manager/README.md) (`cluster_orchestrator/service-manager/`)

They propagate and install the routes to the NetManager components.

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

## From the official release

The [worker installation](../README.md) script (`scripts/InstallOakestraWorker.sh`) installs the NetManager release
alongside the NodeEngine. Each release ships `NetManager_<arch>.tar.gz` next to `NodeEngine_<arch>.tar.gz`.

## Build it on your own

From the `net_manager/build` directory run:

```
./build.sh
./install.sh $(dpkg --print-architecture)
```

`build.sh` builds `bin/amd64-NetManager` and `bin/arm64-NetManager`. Supported architectures are `amd64` and `arm64`.

# Run NetManager

## 1) (Optional) Prepare a config file

You can edit the default one placed in `/etc/netmanager/netcfg.json`

The netcfg file must contain the following fields.

```

{
  "NodePublicAddress": "address",
  "NodePublicPort": "port",
  "ClusterUrl": "url",
  "ClusterMqttPort": "port",
  "Debug": false
}

```

The default configuration file will inherith the Node Public Adress from the default gateway and the Cluster url from the Node Engine. If special NAT setups must be take into account, they can be set in this file. 

## 2) Run the netmanager

The net manager Daemon is automaitcally managed when starting up the NodeEngine. If you want to manually run the NetManager simply use

`sudo NetManager`

> N.b. If you manually run the NetManager you need to point the Node Engine at your externally managed NetManager first: `sudo NodeEngine config network manual` (default socket `/etc/netmanager/netmanager.sock`)

## Development setup
The development setup can be used to test locally the tunneling mechanism without the use of the Cluster orchestrator. This setup requires 2 different machines namely Host1 and Host2.
* the Go version in `go.mod` is required 
* run the install.sh to install the dependencies on each machine 

### Start the netmanager in debug mode 

Simply set `"Debug": true` in `/etc/netmanager/netcfg.json`

### VSCode debug profile

The repository's [`.vscode/launch.json`](../.vscode/README.md) has a **Debug Net Manager** profile. Its pre-launch task
installs the default config into `/etc/netmanager/` (if missing) and switches the NodeEngine to the custom network mode
(`sudo NodeEngine config network manual`), so the NodeEngine talks to your debugged NetManager over
`/etc/netmanager/netmanager.sock` instead of starting the `netmanager` service. Use the **Debug Worker (Node Engine + Net Manager)**
compound to debug both at once. Run `sudo NodeEngine config network auto` to go back to the installed service.

### Run the tests

```
go test -race ./...
```


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
`10.18.0.0/26` (IPv6 `fc00::/120`)

Address where all the containers of this node belong. Each new container will have an address from this space.
Every node uses the same bridge subnetwork: a namespace IP only has to be unique within its node, because
the proxy forwards to the node endpoint of the route it picked rather than looking a namespace IP up
globally. The per-node subnet request (`RequestSubnetworkMqttBlocking`, the cluster `net/subnet` MQTT
handler and the root `GET /api/net/subnet`) is deprecated; the endpoints still answer, with this fixed
subnet, so older NetManagers keep working.

###Prohibited port numbers
A deployed service can't expose the port 50103

## P2P mode (clusterless workers)

The NetManager can also run **without any root or cluster orchestrator**. When the paired NodeEngine
registers with `mode=p2p`, the NetManager joins a SWIM gossip mesh (`hashicorp/memberlist`,
PSK-encrypted, port 50104) and resolves Service IPs from a gossip-replicated service registry
(`p2p/`) instead of the MQTT table query. The registry plugs into the resolver as a backend
(`resolver/backend.go`), and tunnel payloads between nodes are encrypted with a key derived from the
network secret (`proxy/dataEncryption.go`). See [`P2P_MODE.md`](../P2P_MODE.md) for how it works end
to end.
