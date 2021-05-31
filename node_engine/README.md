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

The Node Engine can be started with the startup script: `./start-up.sh <architecture>`.
A virtualenv will be started and the component will start up.
The architecture currently supported are: amd64 or arm-7
The superuser password will be asked

Use `nohup` if you want Node-Engine to run a SSH server after logged out.


E.g. `nohup ./start-up >/dev/null 2>&1 & ` to run `./start-up` in background (also after logging out from ssh connection) and not to create any nohup log files. 
See here https://stackoverflow.com/questions/10408816/how-do-i-use-the-nohup-command-without-getting-nohup-out


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

## Misc

- Sometimes there is a Broken Pipe Error when the node-engine connects to the cluster-manager: https://stackoverflow.com/questions/11866792/how-to-prevent-errno-32-broken-pipe
