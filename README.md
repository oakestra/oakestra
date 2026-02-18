![workflow code style](https://github.com/oakestra/oakestra/actions/workflows/super-linter.yml/badge.svg)
![node artifacts](https://github.com/oakestra/oakestra/actions/workflows/node_engine_artifacts.yml/badge.svg)
![system artifacts](https://github.com/oakestra/oakestra/actions/workflows/root_system_manager_tests.yml/badge.svg)
[![Stable](https://img.shields.io/badge/Latest%20Stable-🎸Bass%20v0.4.400-green.svg)](https://github.com/oakestra/oakestra/tree/v0.4.400)
[![Github Downloads](https://img.shields.io/github/downloads/oakestra/oakestra/total.svg)]()
![Oakestra](res/oakestra-white.png)


**Oakestra** is an orchestration platform designed for Edge Computing.
Popular orchestration platforms such as Kubernetes or K3s struggle at maintaining workloads across heterogeneous and constrained devices. 
Oakestra is build from the ground up to support computation in a flexible way at the edge. 

🌐 Read more about the project at: [oakestra.io](http://oakestra.io)

📚 Check out the project wiki at: [oakestra.io/docs](https://www.oakestra.io/docs/getting-started/welcome-to-oakestra-docs/)

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

Check out the [GetStarted Guide](https://www.oakestra.io/docs/getting-started/oak-environment/create-your-first-oakestra-orchestrator/).


# ⚒️ Build Instructions
### Root Orchestrator 
Build and run your own Root Orchestrator

On a Linux machine first, install Docker and Docker compose v2. 

Configure the a custom address used by the dashboard to reach your APIs. By default it uses the current public IP of the machine where you run the root. 

(optional )`export SYSTEM_MANAGER_URL=<Address of current machine>`


Then clone the repo and run:
```bash
export OAKESTRA_VERSION=develop
./scripts/StartOakestraRoot.sh 
```

> Tip: The `OAK_VERSION` variable can be set to a branch or a specific version. A branch triggers a custom build, while a specific version (E.g., v0.4.401) uses the release images for that version. Check [root-orchestrator/README.md](/root_orchestrator/README.md) for further details.
> Tip: we provide a set of compose override files for every need. Full documentation is available [here](https://www.oakestra.io/docs/manuals/advanced-cluster-setup/#compose-overrides) and the available override files are stored in `/root-orchestrator/override-*.yaml`. To add an override export the overrides file variable `OVERRIDE_FILES`. E.g., `export OAK_OVERRIDES=override-no-dashboard.yml,override-no-network.yml`. 

The following ports are exposed:

- Port 80/TCP - Dashboard 
- Port 10000/TCP - System Manager (Public port, but it also needs to be accessible from the Cluster Orchestrator)
- Port 50052/TCP - System Manager (Needs to be exposed to the Clusters for cluster registration.)
- Port 10099/TCP - Service Manager (This port should be exposed only to the Clusters)
- Port 11103/TCP - Marketplace and addons manager (This port should be available only to the admin)

#### ✅ Everything ok? 
Check the oakestra components using: `docker ps`

### Cluster Orchestrator

For each cluster, we need at least a machine running the clsuter orchestrator. 

- Log into the target machine/vm you intend to use
- Install Docker and Docker compose v2.
- Export the required parameters:


#### Optional
```bash
## Choose a unique name for your cluster
export CLUSTER_NAME=My_Awesome_Cluster

## Optional: Give a name or geo coordinates to the current location. This can either be a string or geo coordinates in the for of LATITUDE,LONGITUDE,RADIUS_IN_METER
# E.g. CLUSTER_LOCATION=51.518776717233244,-0.12612153395857345,2000
export CLUSTER_LOCATION=My_Awesome_Location

## IP address where this root component can be reached to access the APIs
export SYSTEM_MANAGER_URL=<IP address>
# Note: Use a non-loopback interface IP (e.g. any of your real interfaces that have internet access).
# "0.0.0.0" leads to server issues
```
If these variables are not set, the startup script will ask with a prompt.

If you wish to build the cluster orchestrator yourself simply clone the repo and run:
```bash
export OAKESTRA_VERSION=develop
./scripts/StartOakestraCluster.sh 
```

> Tip: The `OAK_VERSION` variable can be set to a branch or a specific version. A branch triggers a custom build, while a specific version (E.g., v0.4.401) uses the release images for that version. Check [cluster-orchestrator/README.md](/cluster_orchestrator/README.md) for further details.
> Tip: we provide a set of compose override files for every need. Full documentation is available [here](https://www.oakestra.io/docs/manuals/advanced-cluster-setup/#compose-overrides) and the available override files are stored in `/cluster-orchestrator/override-*.yaml`. 

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

Configure and install the Node Network Manager, just follow the build guide in this [README](github.com/oakestra/oakestra-net) 

Finally, start the node engine with the following command:

```
sudo NodeEngine -a <IP or URL of the cluster orchestrator, default 0.0.0.0> -d
```



