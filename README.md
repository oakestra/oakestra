![workflow code style](https://github.com/oakestra/oakestra/actions/workflows/super-linter.yml/badge.svg)
![example artifacts](https://github.com/oakestra/oakestra/actions/workflows/node_engine_artifacts.yml/badge.svg)
![example artifacts](https://github.com/oakestra/oakestra/actions/workflows/root_system_manager_tests.yml/badge.svg)
[![Stable](https://img.shields.io/badge/Latest%20Stable-%F0%9F%AA%97%20Accordion%20v0.4.301-green.svg)](https://github.com/oakestra/oakestra/tree/v0.4.301)
[![Github All Releases](https://img.shields.io/github/downloads/oakestra/oakestra/total.svg)]()
![Oakestra](res/oakestra-white.png)


**Oakestra** is an orchestration platform designed for Edge Computing.
Popular orchestration platforms such as Kubernetes or K3s struggle at maintaining workloads across heterogeneous and constrained devices. 
Oakestra is build from the ground up to support computation in a flexible way at the edge. 

🌐 Read more about the project at: [oakestra.io](http://oakestra.io)

📚 Check out the project wiki at: [oakestra.io/docs](http://oakestra.io/docs)

---

## 📕 Requirements 
<a name="requirements"></a>

### Minimum System Requirements
Root and Cluster orchestrator (combined):
- Docker + Docker Compose v2
- 5GB of Disk
- 1GB of RAM
- ARM64 or AMD64 architecture

Worker Node:
- Linux based distro with iptables compatbiliety 
- 50MB of space
- 100MB RAM
- ARM64 or AMD64 architecture

### Network Configuration
Root: 
  - External APIs: port 10000
  - Cluster APIs: ports 10099,10000

Cluster: 
  - Worker's Broker: port 10003
  - Worker's APIs: port 10100

Worker: 
  - P2P tunnel towards other workers: port 50103 

# 🌳 Get Started
<a name="🌳-get-started"></a>

Before being able to deploy your first application, we must create a fully functional Oakestra Root 👑, to that we attach the clusters 🪵, and to each cluster we attach at least one worker node 🍃.

Check out the [GetStarted Guide](http://oakestra.io/docs/getstarted/get-started-cluster/).


# ⚒️ Build Instructions
### Root Orchestrator 
Build and run your own Root Orchestrator

On a Linux machine first, install Docker and Docker compose v2. 

Configure the address used by the dashboard to reach your APIs by running:

`export SYSTEM_MANAGER_URL=<Address of current machine>`


Then clone the repo and run:
```bash
cd root_orchestrator/
docker-compose up --build 
```

The following ports are exposed:

- Port 80/TCP - Dashboard 
- Port 10000/TCP - System Manager (It also needs to be accessible from the Cluster Orchestrator)
- Port 50052/TCP - System Manager (Needs to be exposed to the Clusters for cluster registration.)
- Port 10099/TCP - Service Manager (This port can be exposed only to the Clusters)
### Cluster Orchestrator

For each cluster, we need at least a machine running the clsuter orchestrator. 

- Log into the target machine/vm you intend to use
- Install Docker and Docker compose v2.
- Export the required parameters:

```bash
## Choose a unique name for your cluster
export CLUSTER_NAME=My_Awesome_Cluster

## Optional: Give a name or geo coordinates to the current location. Default location set to coordinates of your IP
#export CLUSTER_LOCATION=My_Awesome_Apartment

## IP address where this root component can be reached to access the APIs
export SYSTEM_MANAGER_URL=<IP address>
# Note: Use a non-loopback interface IP (e.g. any of your real interfaces that have internet access).
# "0.0.0.0" leads to server issues
```

If you wish yo build the cluster orchestrator yourself simply clone the repo and run:
```bash
export CLUSTER_LOCATION=My_Awesome_Apartment #If building the code this is not optional anymore
cd cluster_orchestrator/
docker-compose up --build 
```

The following ports must be exposed:

- 10100 Cluster Manager (needs to be accessible by the Node Engine)

### Worker nodes 

#### Worker nodes - Build your node engine
*Requirements*
- Linux OS with the following packages installed (Ubuntu and many other distributions natively supports them)
  - iptable
  - ip utils
- port 50103 available to all worker nodes

Compile and install the binary with:
```
cd go_node_engine/build
./build.sh
./install.sh $(dpkg --print-architecture)
```

Then configure and install the [NetManager](github.com/oakestra/oakestra-net) and perform the startup as usual. 

