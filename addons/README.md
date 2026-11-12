# Addons

Addons are optional extensions that run as containers next to the Oakestra control plane, for
example a monitoring stack or an extra dashboard page. This Go module has the services that
install and run them and a CLI for publishing them:

| Binary | Image | What it does |
|---|---|---|
| `cmd/marketplace` | `marketplace_manager` | The catalog. Keeps track of where addons come from (sources), syncs them and serves one merged list. Runs on root only, port `11102`. |
| `cmd/addons` | `addons_manager` | Installs, updates and removes addons and keeps their containers running. Runs on root (`11101`) and on every cluster (`11201`). |
| `cmd/addonctl` | - | CLI for addon authors and catalog maintainers: `validate` and `build-index`. |

The dashboard (separate repo) talks to both services. The examples below use `curl` instead.

## Quick start

Register a small addon on your root orchestrator, install it and remove it again. You need a
running stack (`./scripts/StartOakestraFull.sh`) and a shell on the root machine.

Log in to get a token. The system manager creates the user `Admin` with password `Admin` on first
start. Change it on anything that isn't a dev setup.

```bash
TOKEN=$(curl -s -X POST http://localhost:10000/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username": "Admin", "password": "Admin"}' | jq -r .token)
```

Register an addon on the marketplace. A local registration may use an image tag; the marketplace
resolves it to a digest and stores the pinned reference.

```bash
curl -s -X POST http://localhost:11102/api/v1/marketplace/addons \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{
    "name": "hello",
    "version": "0.1.0",
    "description": "Serves the nginx welcome page on port 8080.",
    "permissions": { "host_ports": [8080] },
    "services": [
      { "service_name": "web", "image": "nginx:1.27", "ports": { "80": "8080" } }
    ]
  }'
```

The addon now shows up in the catalog as `local/hello`. Ask the root addons manager to install it.
The first call is expected to fail with `409`: the response contains the full definition and its
hash, and the install only goes through once you send that hash back.

```bash
curl -s -X POST http://localhost:11101/api/v1/addons \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name": "local/hello"}' | tee consent.json

HASH=$(jq -r .definition_hash consent.json)

curl -s -X POST http://localhost:11101/api/v1/addons \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"name\": \"local/hello\", \"accept_definition\": \"$HASH\"}"
```

The answer is `201` with the installed record. Its `_id` is what the other addon endpoints take.
Within a few seconds the reconcile loop starts the container `root_addon-local--hello--web`, and
the status goes from `installing` to `active`:

```bash
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:11101/api/v1/addons | jq '.[] | {_id, name, status}'
curl -s http://localhost:8080
```

To remove it, call `DELETE` twice. The first call stops the addon (status `disabling`, then
`disabled`), the second deletes the record.

```bash
ID=<the _id from above>
curl -s -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:11101/api/v1/addons/$ID
curl -s -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:11101/api/v1/addons/$ID
```

## Writing an addon

An addon is described by a definition: a name, a version, the containers to run and the
permissions they need. The marketplace accepts JSON. Catalog repositories keep one YAML or JSON
file per addon. A YAML example with host mounts and a volume:

```yaml
name: node-metrics
version: 1.2.0
description: Exports host metrics for Prometheus.
homepage: https://github.com/your-org/node-metrics
maintainers:
  - Jane Doe <jane@example.org>

permissions:
  host_paths: ["/proc:ro"]
  host_root: true            # /proc gives root on the host, so it needs this too
  host_ports: [9100]

services:
  - service_name: exporter
    image: quay.io/prometheus/node-exporter@sha256:<digest>
    command: --path.procfs=/host/proc --collector.disable-defaults --collector.cpu
    volumes:
      - /proc:/host/proc:ro
      - data:/var/lib/exporter
    ports:
      "9100": "9100"
    environment:
      LOG_LEVEL: info

volumes:
  - name: data
```

### Fields

| Field | Required | |
|---|---|---|
| `name` | yes | Lowercase letters, digits and `-`, 1 to 32 characters, starting and ending with a letter or digit. |
| `version` | yes | Semantic version (`1.2.0`). Updates are offered when the catalog has a higher one. |
| `description` | no | Up to 4096 characters. |
| `homepage` | no | http(s) URL. |
| `maintainers` | no | Up to 32 strings. |
| `permissions` | no | What the addon may do on the host. Anything the services need has to be declared here, see [Permissions](#permissions). |
| `services` | yes | The containers, see below. |
| `volumes` | no | Named volumes the addon creates: `name`, optional `driver`, `driver_opts`, `labels`. |
| `networks` | no | Networks the addon creates: `name`, optional `driver`, `enable_ipv6`. |
| `ui` | no | A page for the dashboard, see [Addons with a dashboard UI](#addons-with-a-dashboard-ui). |
| `source` | no | Set by the marketplace. Leave it out. |

Each service has:

| Field | |
|---|---|
| `service_name` | Same rules as the addon name. Unique within the addon. |
| `image` | `registry/repo@sha256:<digest>`. Catalogs (official and taps) must pin by digest. Local registrations may use a tag. |
| `command` | One string, split like a shell would (spaces separate, quotes group, `\` escapes). No shell runs it, so no pipes or variables. |
| `volumes` | Docker short syntax: `/host/path:/container[:ro]` for a bind mount, `<volume>:/container[:ro]` for one of the addon's volumes, or `/container` for an anonymous volume. |
| `ports` | Container port (`"80"`, `"80/tcp"`, `"53/udp"`) to host port (`"8080"` or `"127.0.0.1:8080"`). |
| `networks` | Names from the addon's `networks`, `host`, `none`, or `oakestra`. Without any, the service joins a network private to the addon. |
| `environment` | String to string map. |
| `labels` | String to string map. Labels starting with `oak.`, `oakestra.`, `org.oakestra.` or `com.docker.compose.` are reserved. |

Field names are matched exactly. Unknown fields (`privileged`, `cap_add`, a misspelled `Image`)
and duplicate keys are rejected instead of being ignored. In YAML, quote numbers that are used as
strings, such as ports and environment values.

Check a definition before publishing it:

```bash
cd addons
go run ./cmd/addonctl validate path/to/node-metrics.yaml
```

`validate` applies the catalog rules, so tags fail there even though a local registration would
accept them.

### Names in Docker

The catalog shows an addon under its qualified name: `<name>` for the official catalog,
`<tap>/<name>` for a tap and `local/<name>` for local registrations. Docker and DNS can't use `/`,
so containers, volumes and networks are named after the addon id instead: `<name>` for official
addons and `<source>--<name>` for the rest. A container is called
`<plane>_addon-<id>--<service>`, for example `root_addon-local--hello--web`. These names are
always derived from the id, never taken from the definition, so an addon can't claim the name of a
core container or of another addon's volume.

### Permissions

The engine derives what a definition needs and refuses it unless `permissions` covers all of
it. Declaring more than needed is allowed, but the admin sees the whole list when installing.

| Permission | Needed for |
|---|---|
| `host_paths` | Bind mounts from the host, as `/path` or `/path:ro`. Each entry covers exactly that path: `/data` does not cover `/data/sub`. A read-write entry also covers a read-only mount of the same path, not the other way round. |
| `host_ports` | Publishing ports on the host. List every host port. |
| `host_root` | Mounting a path that gives root on the host, even read-only. See below. |
| `docker_socket` | Mounting a container runtime socket (Docker, containerd, CRI-O, Podman) or a directory that contains one, such as `/run` or `/run/docker`. This is root on the host. |
| `host_network` | `networks: [host]`. |
| `core_network` | `networks: [oakestra]`. Mongo, Redis and the internal APIs on that network have no authentication, so this is close to root on the control plane. |

`host_root` is needed for `/`, `/etc`, `/root`, `/boot`, `/proc`, `/sys`, `/dev`, `/usr`, `/bin`,
`/sbin`, `/lib`, `/lib64`, `/home`, the runtime state directories (`/var/lib/docker`,
`/var/lib/containerd`, `/var/lib/kubelet`, `/run/containerd`), any directory above one of those,
and anything below `/etc`, `/root`, `/boot`, `/proc`, `/sys`, `/dev` and the runtime state
directories. Read-only paths like `/usr/share/zoneinfo` or `/lib/modules` only need a
`host_paths` entry.

Docker follows symlinks in a bind source when the container starts. A container that can write to
a host directory could leave a symlink there that redirects a later mount, so there are a few
rules on top of the permissions:

- An addon can't mount a path below one of its own read-write bind mounts. Mount the parent
  instead. Mounting a parent of a read-write path is fine.
- Two addons can't share host paths (same path, or one inside the other) if either of them
  mounts it read-write. The error reads `... conflicts with host path of addon X`. Two read-only
  mounts of the same path are fine. Disabled addons don't count.
- An update can't mount a path below one the previous version had read-write. Mounting the same
  path again is fine.

Some things can't be granted at all: `privileged`, extra capabilities, devices, host PID or IPC
namespaces, reserved labels, and volume `driver_opts` other than plain local or bind options.

### What the engine does to every container

Addon containers run unprivileged, with no added capabilities, `NET_RAW` dropped,
`no-new-privileges`, and capped json-file logs. The addons manager also sets resource limits (see
[Configuration](#configuration)), by default 1 CPU, 512 MB of memory and 1024 processes per
container. Images whose own labels use one of the reserved prefixes are refused.

## Publishing an addon

### Official catalog

The official catalog is built from the `oakestra/community-marketplace` repository. Every addon is
one file named after it (`node-metrics.yaml`), changes go through pull requests, and CI runs
`addonctl validate` on them. After a merge, CI builds `index.json` with `addonctl build-index` and
publishes it to GitHub Pages, where every Oakestra marketplace picks it up on its next sync.

### Your own catalog (tap)

A tap is an index that you host yourself, for example for addons that are internal to your
organisation. Build the index from a directory of definition files:

```bash
go run ./cmd/addonctl build-index --source myorg --serial 7 --out index.json ./catalog
```

`--source` is the tap name the index is meant for. `--serial` must grow with every publication:
the marketplace refuses an index whose serial is lower than the last one it accepted. Host
`index.json` on any https server and add it on the root marketplace:

```bash
curl -s -X POST http://localhost:11102/api/v1/marketplace/sources \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name": "myorg", "url": "https://example.org/oakestra/index.json"}'
```

Its addons then show up as `myorg/<name>`. Optional fields: `token` (sent as a bearer token,
never returned by the API), `allow_insecure` (allow plain http), `allow_private` (allow private and
loopback addresses).

### Index format

An index is a JSON file with `schema` (always `1`), `source`, `serial`, `generated_at`, `addons`
(the definitions) and `signatures` (reserved, empty for now). Indexes are not signed yet. What
protects them today is digest pinning, the serial check and review of the catalog repository.

The marketplace syncs every source every `MARKETPLACE_SYNC_INTERVAL` (6 hours by default) using
ETags, or right away with `POST /api/v1/marketplace/sources/<name>/sync`. If a fetch fails or the
index is rejected, the marketplace keeps serving the last good copy and shows the problem in the
source's `last_error`, next to `last_attempt_at` and `last_synced_at`.

Fetches are https only (unless `allow_insecure`), limited to 5 MiB and 10 seconds, follow
redirects only on the same host, and refuse private, loopback and link-local addresses (unless
`allow_private`), so a tap URL can't point the marketplace at your internal network.

### Without internet access

Set `MARKETPLACE_OFFICIAL_INDEX=off` and upload an index file instead:

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" -F file=@index.json \
  http://<root>:11102/api/v1/marketplace/sources/official/import
```

The serial check still applies. An imported file is only as trustworthy as the way it reached you.
The images have to be available to the Docker daemon on each machine, preloaded or from a local
registry.

## Installing and running addons

### Install, update and approve

Install, update and approve all ask the admin to confirm the exact definition that will run. The
flow is the same for all three:

1. Call the endpoint without `accept_definition`.
2. The addons manager fetches the definition from the marketplace and answers `409` with
   `definition` (the exact JSON that will be stored), `definition_hash` and `permissions`. Show
   these to the admin.
3. Call again with `accept_definition` set to the hash.

The marketplace doesn't publish hashes. Only the hash from the `409` counts, so the
admin always confirms what the addons manager actually fetched. If the catalog changes between the
two calls, the hash no longer matches and you get a new `409`.

Updates always ask, even when the permissions stay the same, because a new version can run
different images. The `409` of an update also has `previous_version`, `previous_permissions` and
`permissions_grow`, so a UI can highlight new permissions. A host path only counts as already
granted when the old version had that exact path with at least the same access.

`approve` is for an addon that is `failed`, for example after an image couldn't be pulled or with
`grant signature invalid; re-approve the addon` (see [Grant signing](#grant-signing-and-the-state-volume)).
It only succeeds if the marketplace still serves the installed version with the same definition.
A failed addon is deployed again, a running one keeps running. If the marketplace has a newer
version, update instead. If the definition changed under the same version, remove and reinstall.

Clusters work the same way: send the requests to the cluster addons manager on port `11201`. It
reads definitions from the root marketplace.

### Statuses

| Status | |
|---|---|
| `installing`, `updating` | Accepted, the reconcile loop is creating containers. |
| `active` | All containers are running. |
| `partially_active` | Some containers are running. `status_details.failed_services` lists the others. |
| `failed` | `status_details.message` says why: a permission or policy violation, a host path conflict, a bad grant signature, an image that can't be pulled. |
| `disabling`, `disabled` | Being stopped, or stopped. A disabled addon can be deleted. |

The reconcile loop runs every `RECONCILE_INTERVAL` (15 seconds). It also checks the stored
definition against the permission rules again before it creates containers, so an addon whose
record no longer passes fails with `the installed definition does not pass the current permission
policy`. Approving it again doesn't help; update to a version that declares what it needs, or
remove it.

Every `UPDATE_CHECK_INTERVAL` (10 minutes) the addons manager compares installed versions with the
catalog and sets `update_available` and `latest_version`.

### Uninstall

`DELETE /api/v1/addons/<_id>` on an addon that isn't disabled sets it to `disabling`; the reconcile
loop removes its containers and networks. A second `DELETE` on the disabled addon removes the
record. Named volumes are left in place, so data survives a reinstall. Remove them with
`docker volume rm` if you don't need them.

## API reference

### Authentication and errors

The services accept the JWT that system manager hands out at `POST :10000/api/auth/login`, sent as
`Authorization: Bearer <token>`. Endpoints marked "admin" need the `Admin` role, "user" means any
valid token. Until a service has loaded the public key to check tokens, protected endpoints answer
`503`.

Errors are JSON: `{"message": "..."}`. Validation errors from the marketplace (`422`) add an
`errors` array of `{"path", "message"}`, for example `{"path": "services[0].image", ...}`. Request
bodies are decoded strictly, an unknown field is a `400` on the addons manager and a `422` on the
marketplace.

### Addons manager (root `11101`, cluster `11201`)

| Method | Path | Auth | |
|---|---|---|---|
| `GET` | `/api/v1/addons[?status=<status>]` | user | Installed addons. When an addon fails, `logs` holds the output of its containers. Only admins get `logs`, since addons may print secrets. |
| `GET` | `/api/v1/addons/<_id>` | user | One installed addon. |
| `POST` | `/api/v1/addons` | admin | Install. Body `{"name": "<qualified name>", "accept_definition": "<hash>"}`. `201`, or `409` for the confirmation, if already installed or on a host path conflict. |
| `POST` | `/api/v1/addons/<_id>/update` | admin | Update to the catalog version. Body `{"accept_definition": "<hash>"}`. `409` if there is no newer version or the addon is busy. |
| `POST` | `/api/v1/addons/<_id>/approve` | admin | Confirm an installed addon again. Body `{"accept_definition": "<hash>"}`. |
| `DELETE` | `/api/v1/addons/<_id>` | admin | Disable (`200`), or delete if already disabled (`204`). |
| `GET` | `/api/v1/ui-addons` | none | Active addons that have a UI. Always empty on clusters. |

`<_id>` is the `_id` of the installed record from the list, not the addon name. Besides the
definition fields, a record has `qualified_name`, `addon_id`, `source`, `status`,
`status_details`, `update_available`, `latest_version`, `created_at` and `updated_at`.

### Marketplace (root `11102`)

Reads need no token: the cluster addons managers read from here and have no user token. Writes
need an admin token.

| Method | Path | Auth | |
|---|---|---|---|
| `GET` | `/api/v1/marketplace/addons[?source=&name=]` | none | All addons from all sources. |
| `GET` | `/api/v1/marketplace/addons/<source>/<name>` | none | One addon. `<source>` is `official`, `local` or a tap name. |
| `POST` | `/api/v1/marketplace/addons` | admin | Register a local addon. Body is the definition. `201`. |
| `DELETE` | `/api/v1/marketplace/addons/local/<name>` | admin | Remove a local addon. `204`. |
| `GET` | `/api/v1/marketplace/sources` | none | Sources with their sync state. |
| `POST` | `/api/v1/marketplace/sources` | admin | Add a tap. Body `{"name", "url", "token"?, "allow_insecure"?, "allow_private"?}`. `201`. |
| `DELETE` | `/api/v1/marketplace/sources/<name>` | admin | Remove a tap. `204`. `official` and `local` can't be removed (`409`). |
| `POST` | `/api/v1/marketplace/sources/<name>/sync` | admin | Sync now. `502` if the fetch or the index fails. |
| `POST` | `/api/v1/marketplace/sources/<name>/import` | admin | Upload an index, as the raw JSON body or as multipart field `file`. |
| `GET` | `/api/v1/marketplace/auth/public-key` | none | The root's JWT public key, `{"public_key": "<PEM>"}`. |

Catalog entries add `qualified_name`, `id` and `required_permissions` (what the definition
actually uses) to the definition. Sources never include their token, only `has_token`, and URLs
are shown without the query string because presigned URLs keep secrets there.

## Addons with a dashboard UI

An addon can add pages to the dashboard. The dashboard loads them at runtime as a Module
Federation remote. Add a `ui` block that names the service serving the UI:

```json
{
  "name": "insights",
  "version": "0.1.0",
  "description": "Cluster insights for the dashboard.",
  "services": [
    { "service_name": "insights-ui", "image": "ghcr.io/your-org/addon-insights-ui@sha256:..." }
  ],
  "ui": { "service": "insights-ui", "sdk": "^0.1.0" }
}
```

`ui.sdk` is the semver range of `@oakestra/dashboard-sdk` the UI was built against. The UI
container serves `mf-manifest.json`, `remoteEntry.js` and `assets/` over plain http on port 80.
The addons manager attaches that service to the `oakestra_addon_ui` network with the alias
`addonui-<id>`, and the dashboard's nginx proxies `/addon-assets/<id>/...` to it. The UI service
doesn't need `core_network` for this. Don't name your addon after a built-in dashboard module
(`applications`, `infrastructure`, `users`, `organizations`, `help`), the dashboard ignores those.

The dashboard asks `GET /api/v1/ui-addons` on the root addons manager which UIs to load:

```json
[{ "name": "insights", "version": "0.1.0", "sdk": "^0.1.0" }]
```

`name` here is the addon id and `version` the installed addon version, which the dashboard also
uses to bust caches. The dashboard checks the SDK range against its own version and won't
load an incompatible UI. That check is for compatibility, not security.

Addon UIs need dashboard-v2 (`github.com/oakestra/dashboard-v2`), which you get with
`override-dashboard-v2.yml` (`override-dashboard-v2.yaml` for 1-DOC). They don't work with
`override-network-host.yml`, because without Docker DNS the dashboard can't resolve
`addonui-<id>`. `override-no-addons.yml` removes the addon services and sets a dashboard preset
that hides the addon pages. For your own presets, see `docs/addons/presets.md` in the dashboard-v2
repo.

## Configuration

Both services read their configuration from environment variables. The compose files in
`root_orchestrator/` and `cluster_orchestrator/` set the ones that differ from the defaults.

### marketplace

| Variable | Default | |
|---|---|---|
| `ADDON_MARKETPLACE_PORT` | `11102` | Listen port. |
| `ADDON_MARKETPLACE_MONGO_URL`, `ADDON_MARKETPLACE_MONGO_PORT` | `localhost`, `10007` | MongoDB. Database `marketplace`, collections `sources` and `catalog`. |
| `MARKETPLACE_OFFICIAL_INDEX` | `https://oakestra.github.io/community-marketplace/index.json` | URL of the official index. `off` disables syncing it, so it can only be fed by import. |
| `MARKETPLACE_OFFICIAL_TOKEN` | unset | Bearer token for the official index, for private development setups. |
| `MARKETPLACE_SYNC_INTERVAL` | `6h` | At least `1m`. |
| `MARKETPLACE_CORS_ORIGIN` | unset | Allowed CORS origin. Unset sends no CORS headers; the dashboard goes through its own nginx and doesn't need them. |
| `MARKETPLACE_REGISTRY_ALLOW_PRIVATE` | `false` | Let digest resolution for local registrations reach registries on private or loopback addresses over plain http. Only for local registries. |
| `JWT_*` | | See [Token verification](#token-verification). |

### addons

| Variable | Default | |
|---|---|---|
| `ADDONS_MANAGER_PORT` | `11101` | Listen port. Compose sets `11201` on clusters. |
| `ADDONS_ENGINE_MONGO_URL`, `ADDONS_ENGINE_MONGO_PORT` | `localhost`, `10007` | MongoDB. Database `addons`, collection `installed`. |
| `MARKETPLACE_ADDR` | `http://localhost:11102` | The root marketplace. On a cluster this points at the root. |
| `ORCHESTRATION_PLANE` | `root` | `root` or `cluster`. Used in container names. Only root serves `/ui-addons`. |
| `RECONCILE_INTERVAL` | `15s` | Go duration or seconds, at least 1s. |
| `UPDATE_CHECK_INTERVAL` | `10m` | Go duration or seconds, at least 1s. |
| `ADDONS_CORS_ORIGIN` | `*` | Allowed CORS origin. |
| `ADDONS_STATE_DIR` | `/var/lib/oakestra-addons` | Where the grant key lives, see below. |
| `ADDON_PIDS_LIMIT` | `1024` | Max processes per container. `0` turns the limit off. |
| `ADDON_MEMORY_LIMIT` | `512m` | Memory per container, bytes with optional `k`, `m` or `g`, at least `6m`. `0` or `unlimited` turns it off. |
| `ADDON_CPU_LIMIT` | `1` | CPUs per container, fractions allowed. `0` turns it off. |
| `LOG_LEVEL` | `info` | |
| `JWT_*` | | See [Token verification](#token-verification). |

### Token verification

Both services need the root's public key to check tokens. They look for it in this order:

1. `JWT_PUBLIC_KEY`: the PEM itself. Literal `\n` sequences are accepted.
2. `JWT_PUBLIC_KEY_URL`: a URL that returns `{"public_key": "<PEM>"}`.
3. `JWT_GENERATOR_URL` and `JWT_GENERATOR_PORT` (default `localhost`, `10011`): the root
   `jwt_generator`.

Root uses the jwt_generator. Clusters have none, so the cluster compose file sets
`JWT_PUBLIC_KEY_URL` to the root marketplace's `/api/v1/marketplace/auth/public-key`.

The jwt_generator creates a new key pair on every restart, so a key loaded from it is fetched again
when a token fails to verify. Keys from `JWT_PUBLIC_KEY_URL` or `JWT_PUBLIC_KEY` are loaded once
and never refreshed. Root to cluster traffic is plain http, and a refresh on failure would let
anyone on the path send a bad token and then answer the refetch with their own key. On clusters,
set `JWT_PUBLIC_KEY` to the root's key where you can, and restart the cluster addons manager after
the root key changes.

## Security notes

### Grant signing and the state volume

Installed addons are stored in Mongo. Someone who can write to Mongo could change a stored
definition and its granted permissions together, for example to add the Docker socket to both. To
catch that, the addons manager signs each record with an HMAC key and only runs records it signed.

The key (`grant.key`) and an `instance.id` live in `ADDONS_STATE_DIR`, a named volume of its own:
`addons_state` on root, `cluster_addons_state` on a cluster. Don't share it with Mongo or with an
addon. The instance id is added to the labels of every container the manager creates, so two
managers on one Docker daemon leave each other's containers alone.

Back this volume up together with the Mongo data. If it is lost, the manager creates a new key and
every installed addon fails with `grant signature invalid` until an admin approves it again. If the
key file exists but can't be read or has the wrong size, the manager refuses to start instead of
creating a new key. The signature doesn't stop someone from restoring an older signed copy of a
record, but an admin approved that copy when it was current.

### What is exposed

- Only the addons manager mounts the Docker socket. The marketplace resolves digests through the
  registry HTTP API and doesn't need it.
- Mongo (10007, 10008, 10107, 10108), Redis (6379, 6479), the resource abstractors (11011, 11012)
  and the jwt_generator (10011) are not published on the host. They have no authentication, and
  the jwt_generator's `POST /create` hands out tokens to anyone. `override-network-host.yml` puts
  them back on the host network, so avoid it where untrusted users or addons can reach the host.
- The marketplace reads are open because clusters on other hosts read from it. Anyone who can
  reach port `11102` can see the configured taps (never their tokens) and their addons. Firewall
  the port if you don't run remote clusters or want the catalog closed.

### Trust

- Indexes aren't signed. Digest pinning, the serial check and review in the catalog repository are
  what you rely on.
- An addon granted `docker_socket`, `host_root` or `core_network` has root on the host
  or the control plane. The confirmation step shows the full definition with image digests, not
  just the permission list, but it is the only check.

## Development

The services are part of the root and cluster compose files and start with the stack. To rebuild
only them, from `root_orchestrator/`:

```bash
docker compose -f docker-compose.yml up --build marketplace_manager root_addons_manager
```

Tests and vet, from `addons/`:

```bash
go vet ./... && go test -race ./...
```

The images are built with `addons/` as the context (`Dockerfile.marketplace`,
`Dockerfile.addons`). The definition format, validation and permission rules live in
`internal/definition`; its package doc lists what the runtime has to do for the rules to hold.
