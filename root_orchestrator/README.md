# Cloud-components

By design, the centralized Root Orchestrator contains

- a System Manager for user interaction with the system
- a Database (mongodb) to store data about the participating clusters
- a Scheduler which calculates task-to-cluster affinity
- some Monitoring solution (first prototype shows read-only Grafana Dashboards which collect metrics from Prometheus instances on each cluster orchestrator)

During implementation, some components could be changed/renamed/replaced/merged.

## Usage

Export the environment variables with the public ip/URL where the root orchestrator will be exposed.

```
export SYSTEM_MANAGER_URL=<IP ADDRESS OF THE NODE HOSTING THE ROOT ORCHESTRATOR>
```

Then se the docker-compose.yml with `docker-compose -f docker-compose.yml up --build` to start the root components.

## Customize deployment

It's possible to use the docker ovveride functionality to exclude or customize the root orchestrator deployment. 

E.g.: Do not deploy the dahboard:

`docker-compose -f docker-compose.yml -f override-no-dashboard.yml up --build`

E.g.: Exclude network component:

`docker-compose -f docker-compose.yml -f override-no-network.yml up --build`


E.g.: Customize network component version

- open and edit `override-custom-serivce-manager.yml` with the correct container image 
- run the orchestrator with the ovverride file: `docker-compose -f docker-compose.yml -f override-custom-service-manager.yml up --build`


