# Root-components

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

Then set the docker-compose.yml with `docker-compose -f docker-compose.yml up --build` to start the root components.

## Custom Library Dependency

Per default, pip will build the python dependencies found `libraries/` (resource_abstractor_client and oakestra_utils_library) from the oakestra github
repository, specifically from the develop branch. To override this, set the environment variable `LIB_BRANCH`.

E.g. you have made changes to the resource_abstractor_client library and wish to test this locally. Push your changes to `XXX-example-library-rework` and set
`LIB_BRANCH` to `XXX-example-library-rework`. Pip will then pull the library from your branch. Note that only pushed changes will have an impact on your local setup.

## Customize deployment

It's possible to use the docker override functionality to exclude or customize the root orchestrator deployment.

E.g.: Do not deploy the dashboard:

`docker-compose -f docker-compose.yml -f override-no-dashboard.yml up --build`

E.g.: Exclude network component:

`docker-compose -f docker-compose.yml -f override-no-network.yml up --build`

E.g.: Customize network component version

- open and edit `override-custom-serivce-manager.yml` with the correct container image
- run the orchestrator with the override file: `docker-compose -f docker-compose.yml -f override-custom-service-manager.yml up --build`

E.g.: Use local development network component

In case you want to use changes made to the root network component in your deployment,
you can use the `override-local-service-manager.yml` override file.

- copy the `oakestra-net/root-service-manager/service-manager` folder to the `root_orchestrator` directory
- run the orchestrator with the override file: `docker-compose -f docker-compose.yml -f override-local-service-manager.yml up --build`

E.g.: Enable IPv6 for container deployments

Usage: `docker-compose -f docker-compose.yml -f override-ipv6-enabled.yml`

This override sets up a bridged docker network, assigning each container a static IPv4+IPv6 address.
Note that the IP protocol version used for connection establishment using hostname resolution depends on the implementation.
Example: IPv6 server receiving IPv4 request -> source address is in 4-to-6 mapped format (http://mars.tekkom.dk/w/index.php/IPv4-Mapped_IPv6_Address)

E.g.: Disable [observability stack](../root_orchestrator/config/README.md)

Usage: `docker-compose -f docker-compose.yml -f override-no-observe.yml`