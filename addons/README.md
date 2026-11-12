# Addons Engine

The addons engine installs optional extensions ("addons") next to the Oakestra control plane and
keeps their containers running. It is one Go module with two services and a CLI for people who
publish addons:

| Binary | Image | Role |
|---|---|---|
| `cmd/marketplace` | `marketplace_manager` | The catalog: addon sources, sync, local registrations, merged listing. Root only. |
| `cmd/addons` | `addons_manager` | Install, update and uninstall API, plus the reconcile loop that runs the addon containers. One per plane (root and cluster). |
| `cmd/addonctl` | - | `validate` and `build-index`, used by the community marketplace CI and by anyone writing an addon. |

The Oakestra dashboard (separate repo) talks to both services directly.

## Start

The services are part of the root and cluster compose files, so they come up with the stack:

```bash
./scripts/StartOakestraFull.sh
```

To work on them only, from `root_orchestrator/`:

```bash
docker compose -f docker-compose.yml up --build marketplace_manager root_addons_manager
```

Run the tests with `go test -race ./...` inside `addons/`. The images are built with
`addons/` as the build context (`Dockerfile.marketplace`, `Dockerfile.addons`).

## Concepts

### Sources

Every addon comes from a source:

| Source | Name in the catalog | What it is |
|---|---|---|
| `official` | `<name>` (bare) | The curated index published by `oakestra/community-marketplace`. Fetched from `MARKETPLACE_OFFICIAL_INDEX`. |
| `tap` | `<tap>/<name>` | A third-party index that an admin added. Credentials are optional and never returned by the API. |
| `local` | `local/<name>` | A definition registered directly on this marketplace with `POST`. |

Addon, tap and service names match `^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`. The catalog name above
is the qualified name. Docker and DNS use the addon id instead, because `/` isn't valid there:
`<name>` for official addons and `<source>--<name>` for the rest. Containers are named
`<plane>_addon-<id>--<service>` and UI addons get the alias `addonui-<id>`. Container, volume and
network names always come from the id, never from the definition, so an addon can't take over
the name of a core service.

### Versions and updates

An index lists the latest version of each addon. An installed addon remembers its exact version
and image digests. The addons engine compares them with the catalog every
`UPDATE_CHECK_INTERVAL`, and every addon in `GET /api/v1/addons` carries `update_available` and
`latest_version`.

Images in official and tap indexes have to be pinned by digest (`image@sha256:...`). Local
registrations may use tags, which are resolved to a digest when registered.

### Index format and sync

An index is a JSON document: `schema` (1), `source`, `serial`, `generated_at`, `addons[]` and a
reserved `signatures[]`. There is no signing in v1. Instead:

- The `serial` must not go down. A lower serial than the last good one is rejected, which is weak
  rollback protection.
- A failed or rejected fetch keeps the last good snapshot. The source shows `last_error`,
  `last_attempt_at` and `last_synced_at`, so a broken tap is visible instead of silently empty.
- Fetches use https only (unless the source sets `allow_insecure`), a 5 MiB cap, a 10 s timeout,
  same-host redirects, and block loopback, link-local, private and similar address ranges (unless
  the source sets `allow_private`). This guards the server-side fetch against SSRF.
- Sync runs every `MARKETPLACE_SYNC_INTERVAL` (default 6h), on demand with
  `POST /api/v1/marketplace/sources/<name>/sync`, and uses ETags.

Digest resolution goes through the registry HTTP API, so the marketplace needs no Docker socket.

### Air-gapped setups

Set `MARKETPLACE_OFFICIAL_INDEX=off` and import an index file instead:

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" -F file=@index.json \
  http://<root>:11102/api/v1/marketplace/sources/official/import
```

An import is only as trustworthy as the channel that carried the file. The same serial rules apply.
Images have to be available to the host's Docker daemon (pre-loaded or from a local registry).

### Permissions model

A definition has to declare every risky capability in `permissions`:

| Permission | Needed for |
|---|---|
| `host_paths` | Host bind mounts (`/path` or `/path:ro`). A grant covers exactly that mount source: `/data` does not cover `/data/sub`. A read-write grant also covers a read-only mount of the same path, not the other way round. |
| `host_root` | Mounting a root-equivalent host path, even read-only. See below. |
| `host_ports` | Publishing ports on the host. |
| `docker_socket` | Mounting a container runtime socket (Docker, containerd, CRI-O, Podman), or any directory above one, such as `/run`, `/run/docker` or `/run/user`. This is root on the host. |
| `host_network` | `network: host`. |
| `core_network` | Joining the `oakestra` network, where Mongo, Redis and the core APIs are reachable without authentication. This is close to root on the control plane. |

`host_root` is needed for `/`, `/etc`, `/root`, `/boot`, `/proc`, `/sys`, `/dev`, `/usr`, `/bin`,
`/sbin`, `/lib`, `/lib64`, `/home`, the container runtime state (`/var/lib/docker`,
`/var/lib/containerd`, `/var/lib/kubelet`, `/run/containerd`), every directory above one of
those, and everything below the trees `/etc`, `/root`, `/boot`, `/proc`, `/sys`, `/dev` and the
runtime state directories. Harmless read-only paths such as `/usr/share/zoneinfo` or
`/lib/modules` only need a `host_paths` entry.

Docker follows symlinks in a bind source when the container starts, so a writable bind source
can be used to redirect later mounts. Hence:

- Inside one addon, a path below one of its own read-write bind sources is rejected. Mount the
  parent instead. Mounting an ancestor of a read-write path is fine.
- Across addons, the engine refuses to install, update, approve or start an addon whose bind
  sources equal, contain or lie below those of another addon that isn't disabled, when at least
  one side is read-write (the message reads `... conflicts with host path of addon X`).
  Two read-only mounts never conflict.
- On an update, the new version can't mount a path strictly below a path the previous version
  had read-write (`UpgradeConflict`), because the old version may have left a symlink behind.
  The same path again is fine.

Things that no permission can grant are rejected outright (for example reserved label prefixes
such as `oak.`, `oakestra.`, `org.oakestra.` or `com.docker.compose.`, and volume `driver_opts`
other than plain bind/local ones). Decoding is strict: unknown fields (`privileged`, `cap_add`),
duplicate keys and keys that differ from the documented spelling in case (`Image` instead of
`image`) are rejected, so a reviewer can't be shown one value while another is decoded.

A service that lists no networks gets an addon-private network, not `oakestra`. Only services that
list `oakestra` (with `core_network` declared) join the core network.

The service named in `ui.service` is also attached to the Docker network `oakestra_addon_ui` with
the alias `addonui-<id>`. The root dashboard container is on that network too, so it can proxy the
UI without any addon sitting on the core network. Definitions can't list `oakestra_addon_ui`
themselves. Compose declares the network; the `addons` binary only creates it if it is missing.

The policy runs when a definition enters the catalog (sync, import, local registration), at
install and update, and again right before the reconcile loop creates containers. A stored
definition that no longer passes the policy fails with `the installed definition does not pass
the current permission policy`. Re-approving doesn't help there: update to a version that
declares what it needs, or remove the addon.

### Container hardening and limits

Every addon container runs unprivileged, without added capabilities, with `NET_RAW` dropped and
`no-new-privileges`, with no devices and no host PID/IPC/user/UTS namespace, and with capped
json-file logs. Images are inspected before the container is created: an image whose own labels
use a reserved prefix is refused (`image ... carries the reserved label ...`), since image labels
would otherwise end up on the container without passing the definition check. An existing volume
or network is only reused when it carries this addon's owner labels (and, for volumes, the same
driver options); otherwise the addon fails with a `volume ...` error such as `exists but is not
owned by this addon` or `exists with a different driver or driver options`.

Limits apply to each addon container and are set on the addons manager:

| Variable | Default | |
|---|---|---|
| `ADDON_PIDS_LIMIT` | `1024` | Max processes. `0` disables. |
| `ADDON_MEMORY_LIMIT` | `512m` | Bytes with optional `k`/`m`/`g`, minimum `6m`. `0` or `unlimited` disables. |
| `ADDON_CPU_LIMIT` | `1` | Number of CPUs, fractions allowed. `0` disables. |

### Install and update API

On the addons manager (root `:11101`, cluster `:11201`):

| Method | Path | Auth | |
|---|---|---|---|
| `GET` | `/api/v1/addons`, `/api/v1/addons/<id>` | user token | Installed addons with `status`, `update_available`. |
| `POST` | `/api/v1/addons` | admin | Install `{"name": "...", "accept_definition": "<hash>"}`. |
| `POST` | `/api/v1/addons/<id>/update` | admin | Update to the catalog version, `{"accept_definition": "<hash>"}`. |
| `POST` | `/api/v1/addons/<id>/approve` | admin | Sign the grant of an installed addon again, `{"accept_definition": "<hash>"}`. |
| `DELETE` | `/api/v1/addons/<id>` | admin | Uninstall. |
| `GET` | `/api/v1/ui-addons` | none | Active addons with a UI (root only). |

Install, update and approve all need `accept_definition`, the hash of the whole definition as the
addons manager would store it: images with their digests, commands, environment, volumes and the
permissions. Without the right hash the call answers `409` with `definition` (the canonical
definition the hash covers), `definition_hash` and `permissions` for the dialog. Show it to the
admin, then repeat the call with the hash. The catalog serves no hash on purpose, only the one
from the `409` counts. If the catalog changes in between, the old hash no longer matches and the
call answers `409` again with the new one. Updates always ask, also when the permissions stay the
same, because a new version can run other images under them. The update answer also contains
`previous_version`, `previous_permissions` and `permissions_grow`, so a UI can point out new
permissions. A path counts as covered only when the old grant names that exact path with at least
the same access. A host path conflict with another addon or with the previous version answers
`409`. There is no `PATCH`: state changes are internal to the reconcile loop. Request bodies with
unknown fields, such as the old `accept_permissions`, answer `400`.

Use `approve` when an addon is `failed` with `grant signature invalid; re-approve the addon` (or
`missing`), and to retry a failed addon. It asks for the definition hash again and only goes through
if the marketplace still serves the installed version with the same definition, so a record that
was rewritten in the database can't be approved by accident. A failed addon is deployed again, a
running one keeps running. If the marketplace lists a different version, update instead; if the
definition differs, remove and reinstall.

Statuses: `installing`, `updating`, `disabling`, `disabled`, `failed`, `active`,
`partially_active`. A `failed` addon carries the reason in `status_details`, for example a
permission or policy violation, a bind conflict, a bad grant signature, or an image that can't be
pulled.

Unknown paths and wrong methods answer JSON (`{"message": ...}`) with `404` and `405`, on both
services.

### Grant signing and the state volume

The record of an installed addon lives in Mongo, and whoever can write there could rewrite the
stored definition and the grant together (say, add the docker socket to both). So the engine signs
each record with an HMAC-SHA256 key and only runs records it signed itself.

The key (`grant.key`) and an engine `instance.id` live in `ADDONS_STATE_DIR` (default
`/var/lib/oakestra-addons`), which is a named volume of its own: `addons_state` on root,
`cluster_addons_state` on a cluster. It is created with mode 0700 and must not be shared with
Mongo or an addon. The instance id labels the containers an engine created, so two engines on one
Docker daemon don't touch each other's.

Back this volume up together with the Mongo data. If it is lost or replaced, the engine makes a
new key and every installed addon fails with `grant signature invalid` until you approve it again.
A key file that exists but is unreadable or has the wrong length stops the engine from starting
rather than silently making a new key. The signature does not stop someone from restoring an
older signed state of the same record, but that stays within what an admin approved at some point.

### Marketplace API

On the marketplace (`:11102`). Reads are open, writes need an admin token.

| Method | Path | Auth |
|---|---|---|
| `GET` | `/api/v1/marketplace/addons[?source=&name=]`, `/api/v1/marketplace/addons/<source>/<name>` | none |
| `POST` | `/api/v1/marketplace/addons` (local registration) | admin |
| `DELETE` | `/api/v1/marketplace/addons/local/<name>` | admin |
| `GET` | `/api/v1/marketplace/sources` | none |
| `POST` | `/api/v1/marketplace/sources` (add a tap) | admin |
| `DELETE` | `/api/v1/marketplace/sources/<name>` | admin |
| `POST` | `/api/v1/marketplace/sources/<name>/sync` | admin |
| `POST` | `/api/v1/marketplace/sources/<name>/import` (raw JSON or multipart `file`) | admin |
| `GET` | `/api/v1/marketplace/auth/public-key` | none |

Source listings never contain credentials, and URLs are shown without their query string
(`?<redacted>`), because presigned or `?token=` URLs carry secrets. Error messages are redacted the
same way.

## Addons with a dashboard UI

An addon can add pages to the Oakestra dashboard. The dashboard loads them at runtime as a Module
Federation remote. Add a `ui` block to the definition:

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

`ui.service` must be one of the addon's services and `ui.sdk` is the semver range of the dashboard
SDK the UI was built against. The UI container serves `mf-manifest.json`, `remoteEntry.js` and
`assets/` over plain HTTP on port 80. The dashboard's nginx proxies `/addon-assets/<id>/...` to
`http://addonui-<id>:80/...` over the `oakestra_addon_ui` network. Don't use the name of a
built-in dashboard module (`applications`, `infrastructure`, `users`, `organizations`, `help`): the
dashboard ignores such an addon.

The SDK range check is a usability feature, not a security boundary. The dashboard knows its own
SDK version, computes compatibility and blocks the install. The backend only stores and serves the
range.

To find out which UIs to load, the dashboard calls `GET /api/v1/ui-addons` on the root addons
manager (without a token). It returns every `active` addon with a `ui` block:

```json
[{ "name": "insights", "version": "0.1.0", "sdk": "^0.1.0" }]
```

Addon UIs need dashboard-v2 (`github.com/oakestra/dashboard-v2`). The stack starts v1 by default,
so add `override-dashboard-v2.yml` (`override-dashboard-v2.yaml` for 1-DOC). They don't work with
`override-network-host.yml`, because there is no Docker DNS to resolve `addonui-<id>`.

`override-no-addons.yml` removes the addons engine and sets a dashboard preset (`PRESET_URL`) that
turns off addon discovery and the addon pages. For your own presets, see `docs/addons/presets.md`
in the dashboard-v2 repo.

## Configuration

### marketplace

| Variable | Default | |
|---|---|---|
| `ADDON_MARKETPLACE_PORT` | `11102` | Listen port. |
| `ADDON_MARKETPLACE_MONGO_URL` / `_PORT` | `localhost` / `10007` | MongoDB. Database `marketplace`, collections `sources` and `catalog`. |
| `JWT_GENERATOR_URL` / `JWT_GENERATOR_PORT` | `localhost` / `10011` | Where to fetch the RS256 public key (`GET /key`). |
| `JWT_PUBLIC_KEY_URL`, `JWT_PUBLIC_KEY` | unset | Alternative key sources, see below. |
| `MARKETPLACE_OFFICIAL_INDEX` | the community marketplace URL | Official index (URL or file). `off` disables it. Empty means the default. |
| `MARKETPLACE_OFFICIAL_TOKEN` | unset | Bearer token for the official index, for private development setups. |
| `MARKETPLACE_SYNC_INTERVAL` | `6h` | Minimum `1m`. |
| `MARKETPLACE_CORS_ORIGIN` | unset | Allowed CORS origin. Empty means none. |
| `MARKETPLACE_REGISTRY_ALLOW_PRIVATE` | `false` | Let digest resolution talk to registries on private or loopback addresses, over plain http. Only for local registries in air-gapped or dev setups. |

### addons

| Variable | Default | |
|---|---|---|
| `ADDONS_MANAGER_PORT` | `11101` | Listen port (`11201` on the cluster plane in compose). |
| `ADDONS_ENGINE_MONGO_URL` / `_PORT` | `localhost` / `10007` | MongoDB. Database `addons`, collection `installed`. |
| `MARKETPLACE_ADDR` | `http://localhost:11102` | The root marketplace. The cluster plane points at root's. |
| `ORCHESTRATION_PLANE` | `root` | `root` or `cluster`. Prefixes container names and decides who serves `/ui-addons`. |
| `RECONCILE_INTERVAL` | `15s` | Go duration or plain seconds, at least 1s. |
| `UPDATE_CHECK_INTERVAL` | `10m` | Go duration or plain seconds, at least 1s. |
| `ADDONS_CORS_ORIGIN` | `*` | Allowed CORS origin. |
| `ADDONS_STATE_DIR` | `/var/lib/oakestra-addons` | Grant key and instance id, see above. |
| `ADDON_PIDS_LIMIT`, `ADDON_MEMORY_LIMIT`, `ADDON_CPU_LIMIT` | `1024`, `512m`, `1` | Per-container limits, see above. |
| `LOG_LEVEL` | `info` | |
| `JWT_*` | | Same key loading as the marketplace. |

Key loading order (both binaries): `JWT_PUBLIC_KEY` (static PEM, `\n` sequences accepted) wins,
then `JWT_PUBLIC_KEY_URL`, then `JWT_GENERATOR_URL`/`JWT_GENERATOR_PORT`. The cluster plane has no
jwt_generator, so compose sets
`JWT_PUBLIC_KEY_URL=http://<root>:11102/api/v1/marketplace/auth/public-key`.

Only the jwt_generator source re-fetches the key after a token fails verification: it makes a new
key pair on every restart, and the path is the local Docker network. A key from a URL or from
`JWT_PUBLIC_KEY` is loaded once, because root to cluster is plain HTTP and refreshing would let
anyone on the path trigger a refetch with a bad token and answer it with their own key. So on
clusters, pin `JWT_PUBLIC_KEY` to the root's public key where you can, and restart the addons
manager after a root key rotation. An unparsable `JWT_PUBLIC_KEY` stops loading with an error
instead of retrying.

## Security model

- Authentication: every write, and every addons manager read except `/ui-addons`, needs an RS256
  JWT signed by the key of the root `jwt_generator`. Writes also need the `Admin` role. Until the
  key is loaded, protected endpoints answer `503` instead of letting requests through.
- Policy and permissions: checked in the marketplace when a definition enters the catalog, in the
  addons manager at install and update (with the admin accepting the definition hash), and again
  right before containers are created.
- Naming: containers, volumes and networks are named from the addon id, never from the definition.
- Grants: signed with a key in the state volume, see "Grant signing and the state volume".
- Containers: unprivileged, resource limited, reserved image labels refused, host path overlaps
  between addons refused.
- Docker socket: only the `addons` binary mounts it. The marketplace and the dashboard don't.
- Source fetches: SSRF guards (scheme, size, timeout, redirects, address ranges), serial
  monotonicity and last-good snapshots, see "Index format and sync".
- Internal ports are not published on the host: Mongo (10007, 10008, 10107, 10108), Redis (6379,
  6479), the resource abstractors (11011, 11012) and `jwt_generator` (10011). They have no
  authentication, and `jwt_generator`'s `POST /create` mints tokens for anyone. Only containers on
  the `oakestra` network reach them. `override-network-host.yml` puts them on the host.

Known tradeoffs:

- The marketplace `GET` endpoints are unauthenticated, because the cluster addons manager reads
  root's marketplace across hosts and has no user token. Tap metadata (never credentials) is
  therefore visible to anyone who can reach port 11102.
- There is no signing. Digest pinning, serials and PR review in the community marketplace are the
  trust steps. An offline import is as trustworthy as the way the file was delivered.
- Even with declared permissions, an admin who accepts a broad permission set (`docker_socket`,
  `core_network`) hands that power to the addon. The confirmation dialog is the only guard there.
  It shows the whole definition, image digests included, not just the permission list.
