# Node Engine

The Node Engine software can run on any hardware with an operating system. Currently supported and tested: Linux Distributions such as Raspbian, Ubuntu, Linux Mint. Network Heterogeneity is supported.

## Purpose of Node-Engine

- provides endpoints to start/stop Docker container to be called by cluster-manager (control channel)
- sends Node information (cpu, memory, attached sensors) via MQTT over information channel
- provides endpoint to get information about the Docker container (via Docker API)
- should support technologies other than Docker as well

## Ingoing Requests e.g. from a Cluster Manager

- Node Engine currently does not used REST API endpoints. The control commands are pulled via MQTT

## Outgoing Requests to other components

- Node Engine registers at the Cluster Manager via HTTP-Upgrade Websocket over /init SocketIO namespace
- Node Engine sends regularly cpu, memory, etc. information to the Cluster MQTT Broker
- Node Engine subscribes to control command channel at MQTT to get deploy/delete commands

## Start the Node Engine

The Node Engine can be started with or without the networking component

### Start the Node Engine & NetManager

**It is mandatory to install the NetManager first, checkout [edgeionet](https://github.com/edgeIO/edgeionet/tree/main/node-net-manager)**

export the following environment variables:
- CLUSTER_MANAGER_IP: `export CLUSTER_MANAGER_IP=<cluster_orchestrator_ip>` - public address of the cluster orchestrator
- CLUSTER_MANAGER_PORT: `export CLUSTER_MANAGER_IP=<cluster_orchestrator_ip>` - OPTIONAL, default: 10000
- MQTT_BROKER_URL: `export MQTT_BROKER_URL=<ip or url of the cluster mqtt broker>` - OPTIONAL, default==CLUSTER_MANAGER_IP
- MQTT_BROKER_PORT: `export MQTT_BROKER_PORT=<port of the cluster mqtt broker>` - OPTIONAL, default: 10003
- PUBLIC_WORKER_IP: `export PUBLIC_WORKER_IP=<ip or hostname>` - address where the node is publicly accessible 
- PUBLIC_WORKER_PORT: `export PUBLIC_WORKER_PORT=<public node port>` - port where the node is publicly accessible. OPTIONAL, default: 50103

Then startup the node engine and the NetManager together using: `./start-up-net.sh`.
A virtualenv will be started and the components will start up.
The superuser password will be asked

### Start the Node Engine alone
export the following environment variables:
- CLUSTER_MANAGER_IP: `export CLUSTER_MANAGER_IP=<cluster_orchestrator_ip>` - public address of the cluster orchestrator
- CLUSTER_MANAGER_PORT: `export CLUSTER_MANAGER_IP=<cluster_orchestrator_ip>` - OPTIONAL, default: 10000

Then startup the node engine and the NetManager together using: `./start-up-no-net.sh`.
A virtualenv will be started and the components will start up.

## Get GPU Informtion of a Node

sudo lshw -C display


## Message Format

This json based message format is going from cluster manager to cluster scheduler, and back, and then to the node engine. Coming from System manager.

The message format is not used/ implemented currently. Feel free to edit/extend the internal message format and to use always to use the same format between all components.

```json
{
'command': 'deploy|delete|move|replicate',
'job': {
        'id': 'int',
        'name': 'job_name',
        'image': 'image_address',
        'technology': 'docker|unikernel',
        'etc': 'etc.'  
        },
'cluster': ['list', 'of', 'cluster_ids'],
'worker': ['list', 'of', 'worker_ids']
}
```

## Built With

- Python3.8
  - psutil
  - Flask
  - Flask-MQTT
  - docker==3.4.1
  - APScheduler
  - python-socketio[client]
  - GPUtil
  - tabulate
  - requests
- Golang 1.13 
  - github.com/ghodss/yaml 
  -	github.com/google/gopacket 
  -	github.com/gorilla/mux 
  -	github.com/milosgajdos/tenus 
  -	github.com/songgao/water 
  -	github.com/tkanos/gonfig 
  -	gopkg.in/yaml.v2 

## NetManager

The net manager is the component that carries out the network configuration and create the overlay across all the instances. 
This component install a proprietary bridge in the system and place the proxy-bridge process as a TUN device. It maintains internally a se of caches that are 
used to resolve the network call of the containers. More info are available in `NetManager/docs`. 

## Misc

- Sometimes there is a Broken Pipe Error when the node-engine connects to the cluster-manager: https://stackoverflow.com/questions/11866792/how-to-prevent-errno-32-broken-pipe
