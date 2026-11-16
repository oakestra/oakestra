# Cluster Service Manager

The service manager manages the networking aspects of each job and node. 

Each Node belongs to a subnetwork and upon deployment each job contains belongs to the node's subnetwork. This service manages the discovery and deployment of both the jobs and the nodes. 

## Purpose of Service Manager

- provides a new subnetwork during the node deployment phase
- updates the node namesapce and host addresses to enable the networking
- provides endpoint to solve the cluster-level table query

## Incoming Endpoints which can be used e.g. by the System Manager

- GET /api/net/job/<job_name>/instances table query by job name
- GET /api/net/job/ip/<service_ip>/instances table query by service ip
- GET /api/net/subnet/<node_id> get subnet upon node deployment
- POST /api/net/service/net_deploy_status update service deployment status 

## Outgoing Endpoints to other components

- Root service manager - get subnetwork
- Root service manager - root table query
- Root service manager - update service status

## Run it

The Cluster Service Manager runs as the `cluster_service_manager` container of the cluster orchestrator and is built from
this folder by `cluster_orchestrator/docker-compose.yml`. To run it on its own for development, use the
**Debug Cluster Service Manager** profile in [`.vscode/launch.json`](../../.vscode/README.md).

Run the tests with:

```
pip install -r requirements.txt pytest
pytest
```

## Built With

- Python 3.10
  - Flask
  - bson
  - Flask-PyMongo
  - requests
