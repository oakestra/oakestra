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

## Shared Library Dependency

The shared python libraries (`resource_abstractor_client` and `oakestra_utils_library`) are **always built from this repo's local `libraries/` folder** — nothing is fetched from a remote git repo. The Docker build wires them in via a named build context declared in `docker-compose.yml` (`additional_contexts: libraries=../libraries`), so `docker compose up --build` just works.

To test local changes to a library, edit the code under `libraries/` and rebuild the image (`docker compose up --build`) — no branch pushing required. This requires Docker BuildKit/Buildx (the default on modern Docker).

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

E.g.: Federated login only

Users sign in through an external identity provider instead of an Oakestra password. Off by default; without this override nothing changes.

```
export FEDERATED_LOGIN_AUTHORIZE_URL=<identity provider endpoint>
export FEDERATED_LOGIN_CLUSTER_ID=<id of this cluster at the provider>
export FEDERATED_LOGIN_CLUSTER_SECRET=<secret of this cluster at the provider>
docker compose -f docker-compose.yml -f override-federated-login.yml up -d
```

- Clients exchange a provider token for Oakestra tokens with `POST /api/auth/federated {"token": "..."}`. `GET /api/auth/methods` tells clients which login methods are on.
- The system manager calls `POST $FEDERATED_LOGIN_AUTHORIZE_URL` with `Authorization: Bearer <provider token>` and `{"cluster_id", "cluster_secret"}`, and expects `200 {"username", "email", "roles": [Oakestra roles]}`. Users are created on first login (without password) and their roles are synced on every login. A federated identity can never take over an existing local user.
- Password login, registration and password reset return 403. Refresh tokens from federated login last `FEDERATED_LOGIN_REFRESH_MINUTES` (default 60).
- The JWT generator port (10011) is no longer published on the host (requires Docker Compose >= 2.24.4). It does not apply with `override-network-host.yml`, where every container listens on the host directly.
