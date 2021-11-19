# Root Service Manager

This component is the point of reference for all the cluster service managers. 

## Purpose and Tasks of Root Service Manager

- It generates the subnetworks for the nodes attached to each new cluster 
- It generates the ServiceIP and InstanceIP for each service and the respective instances
- It answers the TableQuery for the address resolution of each service

### Ingoing Endpoints

- Service and Instances deployment endpoints
  - `/api/net/service/net_deploy_status` upon completion of the deployment the worker node network engine  report the assigned namespace and host ip and the tunnel exposed host port.
  - `/api/net/service/deploy` the root orchestrator notifies a new service deployment (with 0 instances) to the network plugin.
  - `/api/net/instance/deploy` the root orchestrator notifies a new scheduled instance for a specific service deployment and the target cluster.
- Table query resolution endpoints 
  - `/api/net/service/<service_name>/instances` service addresses and location query by service name  
  - `/api/net/service/ip/<service_ip>/instances` service location, instance and namespace addresses query by service ip
- Subnetwork management endpoints 
  - `/api/net/subnet` request of a new node subnetwork address
  

## Start the System Manager

####TODO

## Built With

Python3.8 
  - bson
  - Flask
  - Flask_PyMongo
  - Flask-SocketIO
  - Cerberus

The System Manager could be written in another programming language as well. Just the endpoints, protocols, and database API should be supported by the language.