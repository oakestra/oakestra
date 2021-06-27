# EdgeIO Infrastructure Getting Started

## Root Orchestrator setup

On a Linux machine with public IP address or DNS name, first install Docker and Docker-compose. Then, run the following commands to set up the Root Orchestrator components. Open the following ports:

- Port 80 - Grafana Dashboard
- Port 10000 - System Manager


```bash
cd root_orchestrator/
docker-compose up -d
```

## Cluster Orchestrator(s) setup

On a second Linux machine with public IP address or DNS name, first install Docker and Docker-compose. Then, run the following commands to set up the Root Orchestrator components. Open port 10000 for the cluster manager.

- First export the required parameters:
  - export SYSTEM_MANAGER_URL=" < ip address of the root orchestrator > "
  - export CLUSTER_NAME=" < name of the cluster > "
  - export CLUSTER_LOCATION=" < location of the cluster > "


```bash
cd cluster_orchestrator/
docker-compose up -d
```

## Add worker nodes (run Node Engine)

On an arbitrary Linux machine, install Python3.8 and virtualenv. Set the IP address of the cluster orchestrator which should take care of the worker node in the start-up.sh file, and run the following:

```bash
cd node_engine/
./start-up.sh
```

## Application Deployment

Once you have a running edgeIO setup, use the API of the System Manager to deploy applications.
