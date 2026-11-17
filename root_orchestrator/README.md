# Root-components

By design, the centralized Root Orchestrator contains

- a System Manager for user interaction with the system
- a Database (mongodb) to store data about the participating clusters
- a Scheduler which calculates task-to-cluster affinity
- a Root Service Manager (`service-manager/`), the network plugin that assigns service IPs and propagates routes to the clusters
- some Monitoring solution (first prototype shows read-only Grafana Dashboards which collect metrics from Prometheus instances on each cluster orchestrator)

During implementation, some components could be changed/renamed/replaced/merged.

## Usage

Export the environment variables with the public ip/URL where the root orchestrator will be exposed.

```
export SYSTEM_MANAGER_URL=<IP ADDRESS OF THE NODE HOSTING THE ROOT ORCHESTRATOR>
```

Then set the docker-compose.yml with `docker-compose -f docker-compose.yml up --build` to start the root components.

## Shared Library Dependency

The shared python libraries (`resource_abstractor_client` and `oakestra_utils_library`) are **always built from this repo's local `libraries/` folder** — nothing is fetched from a remote git repo. The Docker build wires them in via a named build context declared in `docker-compose.yml` (`additional_contexts: libraries=../libraries`), so `docker compose up --build` just works.

To test local changes to a library, edit the code under `libraries/` and rebuild the image (`docker compose up --build`) — no branch pushing required. This requires Docker BuildKit/Buildx (the default on modern Docker).

## Customize deployment

It's possible to use the docker override functionality to exclude or customize the root orchestrator deployment.

E.g.: Do not deploy the dashboard:

`docker-compose -f docker-compose.yml -f override-no-dashboard.yml up --build`

E.g.: Exclude network component:

`docker-compose -f docker-compose.yml -f override-no-network.yml up --build`

E.g.: Use local changes to the network component

The Root Service Manager is built from [`service-manager/`](service-manager/README.md) like every other root component,
so `docker compose -f docker-compose.yml up --build` already includes your local changes. No extra override is needed.

E.g.: Enable IPv6 for container deployments

Usage: `docker-compose -f docker-compose.yml -f override-ipv6-enabled.yml`

This override sets up a bridged docker network, assigning each container a static IPv4+IPv6 address.
Note that the IP protocol version used for connection establishment using hostname resolution depends on the implementation.
Example: IPv6 server receiving IPv4 request -> source address is in 4-to-6 mapped format (http://mars.tekkom.dk/w/index.php/IPv4-Mapped_IPv6_Address)

E.g.: Disable [observability stack](../root_orchestrator/config/README.md)

Usage: `docker-compose -f docker-compose.yml -f override-no-observe.yml`