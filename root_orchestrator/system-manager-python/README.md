# System Manager

The System Manager is one of the components of the Root Orchestrator. It has an overview of all participating clusters and communicates with the Cloud Scheduler to decide a task placement.

## Purpose and Tasks of System Manager

- It maintains the database: add cluster, remove cluster, count up/down workers within clusters.
- Users can ask the System Manager to deploy a service within the system. The System Manager gives feedback about the status of a job.
- Users can also ask the System Manager to move/migrate a service to another cluster or to another worker node as well as terminate a job, or replicate (scale up, scale down) a job

### Ingoing Endpoints

- provides `/register` endpoint for clusters to register into the system
- provides endpoints to count up / count down the number of participating worker nodes within a cluster
- provides endpoints to move, replicate, terminate a service

### Outgoing Endpoints

- asks Scheduler to calculate placement for a job
- propagates scheduler decision to the selected cluster

## Start the System Manager

Please start the system-manager with `./start-up.sh`.

A virtualenv will be started and the component will start up.

For how to use the endpoints, look in `gitlab.lrz.de/cm/2020-mehdi-masters-thesis/src/README.md`. 

## Built With

Python3.8 
  - bson
  - Flask
  - Flask_PyMongo
  - Flask-SocketIO
  - eventlet
  - PyYAML
  - requests
  - Cerberus

The System Manager could be written in another programming language as well. Just the endpoints, protocols, and database API should be supported by the language.

## Some further Thoughts

- Very interesting (=weird) behaviour: Pycharm 2020.1 reports `socketio.exceptions.BadNamespaceError: /init is not a connected namespace.` when a socketIO client wants to emit a Websocket message to an already connected SocketIO server. But in Pycharm2020.3 it works fine. Not examined further, but, probably, different versions of Python packages are installed on both.
- Not sure if it is a Pycharm Issue, because when inserting time.sleep() instructions before/after socketIO operations, it seems to work.

# Deploy and Undeploy a container 

## Deployment descriptor

In order to deploy a container a deployment descriptor must be ppassed to the deployment command. 
The deployment descriptor contains all the information that EdgeIO needs in order to achieve a complete
deploy in the system. 

`deployment_descriptor_model.yaml`
```yaml
api_version: v0.1 
app_name: demo   
app_ns: default
service_name: service1
service_ns: test
image: docker.io/library/nginx:alpine
image_runtime: docker
port: 80
cluster_location: hpi
node: vm-20211019-009
requirements:
    cpu: 0 # cores
    memory: 100  # in MB
    node: vm-20211019-009
```

- api_version: by default you can leave v0.1
- Give your application a fully qualified name: A fully qualified name in EdgeIO is composed of 4 components
    - app_name: name of the application
    - app_ns: namespace of the app, used to reference different deployment of the same applciation. Examples of namespace name can be `default` or `production` or `test`
    - service_name: name of the service that is going to be deployed
    - service_ns: same as app ns, this can be used to reference different deployment of the same service. Like `v1` or `test`
- image: link to the docker image that will be downloaded 
- image_runtime: right now the only stable runtime is `docker`
- port: port exposed by your service, if any, otherwise leave it blank.
- cluster_location: if you have any preference on a specific cluster write here the name otherwise remove this field.
- node: if you have any prefrence on a specific node within a cluster write here the hostname otw remove this field
- requirements: does your container have any min cpu or memory requirement?
    - cpu: expressed in number of cores
    - memory: expressed in MB
    - node: same value expressed in the node field out of the requirements section. Right now this is a duplicate. And must be included if you specified a node requirement before. Please refer to: [Github Issue #24](https://github.com/edgeIO/src/issues/24)
    
    
## Deploy

After creating a deployment descriptor file, simply deploy a service using 

```
curl -F file=@'deploy.yaml' http://localhost:10000/api/deploy -v
```

deploy.taml is the deployment descriptor file

If the call is successful you'll receive the job name for this service. Save this name for future call.

## Undeploy 

```
curl localhost:10000/api/delete/<job_name>
```

job_name is the name you receive as answer after a deployment 

## Query job status 

```
curl localhost:10000/api/job/status/<job_id>
```

## List of the current deployed Jobs

```
curl localhost:10000/api/jobs
```

Use this endpoint to get information regarding the deployed services in the system. Like the network address assigned to a service.

# Networking 

After a successful deployment in EdgeIO a service is going to have 2 internal IP addresses that can be used for container to container communication. 
The addresses currently can be retrieved with the ```job/list``` command. Each service is going to have an Instance address and a RR address. 

- Instance address is bounded to the specific instance of a service. THis address will always refer to that instance.
- The RR address instead can be used to balance across all the instaces of the same service, "one address to rule them all".

The addresses right now are assigned at deploy time randomly. 
