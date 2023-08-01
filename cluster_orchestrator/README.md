# Cluster

By our design, a Cluster Orchestrator contains:

- a message broker (MQTT)
- a scheduler
- a Cluster Manager
- a database (mongoDB)

The edge nodes push cpu+memory data to the mqtt-broker.


## Message Format between cluster components

As a worker node, to register at the cluster manager / to be registerd by a cluster manager, the following json based message format is used.

```json
{
'id': 'int_id',
'name': 'name',
'ip': 'ip-address',
'port': 'port number',
}
```

mqtt data to publish cpu/memory information from worker to cluster manager via topic `nodes/id/information`:

```json
{
'cpu': 'int_id',
'memory': 'name'
}
```

mqtt data to publish control commands from CO to worker via topic `nodes/id/controls`:

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
'cluster': 'cluster_id' (optional),
'worker': 'worker_id' (optional)
}
```

json based HTTP message from cluster manager to cluster scheduler:

- job description coming from system-manager


HTTP scheduling answer from scheduler back to cluster manager. A list of workers who are contacted

```json
{
'workers': ['list', 'of', 'worker_ids'],
'job': {
    'image': 'image_url'
  }
}
```



## Usage

- First export the required parameters:
  - export SYSTEM_MANAGER_URL=" < ip address of the root orchestrator > "
  - export CLUSTER_NAME=" < name of the cluster > "
  - export CLUSTER_LOCATION=" < location of the cluster > "

- Use the docker-compose.yml with `docker-compose -f docker-compose.yml up --build` to start the cluster components.

N.b. if you're using docker compoe with **sudo** don't forget to use the -E flag E.g., **sudo -E docker-compose etc..**. This will export the env variables. 

## Customize deployment

It's possible to use the docker ovveride functionality to exclude or customize the cluster orchestrator deployment. 

E.g.: Exclude network component:

`docker-compose -f docker-compose.yml -f override-no-network.yml up --build`


E.g.: Customize network component version

- open and edit `override-custom-serivce-manager.yml` with the correct container image 
- run the orchestrator with the ovverride file: `docker-compose -f docker-compose.yml -f override-custom-service-manager.yml up --build`

