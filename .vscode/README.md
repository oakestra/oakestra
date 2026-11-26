# VS Code Debug Profiles

This folder wires up **run/debug configurations** ([launch.json](launch.json)) and their
supporting **tasks** ([tasks.json](tasks.json)) so you can run any single Oakestra
component locally under the debugger while the rest of the stack runs in Docker.

## How it works

Each debug profile has a `preLaunchTask` that, before your breakpoints go live:

1. **Brings up the required orchestrator stack** in Docker (`root-orchestrator-up`, and
   `cluster-orchestrator-up` for cluster components). The stack is started with
   `override-network-host.yml` + `override-custom-service-manager-version.yml`.
2. **Removes the one container** you're about to replace (e.g. `remove-root-manager`), so
   the locally-debugged process can take over its port. Everything the component depends on
   keeps running from its Docker container.
3. **Installs that component's dependencies** (Python profiles only), via
   `install-*-dependencies`.

Because the stack runs with **host networking**, every host value in the launch envs is
`0.0.0.0` (container DNS names don't resolve from the host).

> **Requirements:** Docker + Buildx, a Python interpreter selected in VS Code (for
> `debugpy` profiles), the Go extension (for `go` profiles), Node.js/npm (for the Angular
> dashboard), and root/sudo for the Go worker/CSI profiles (`asRoot`).

## Profiles

### Worker

| Profile | Type | Component | Notes |
|---|---|---|---|
| **Debug Node Engine** | Go | `go_node_engine` | Runs the node daemon (`-a 0.0.0.0`); brings up root + cluster and registers the local NodeEngine against the cluster. Runs as root. |
| **Debug Node Engine With Custom Cluster** | Go | `go_node_engine` | Same, but prompts for the cluster IP (`enterClusterIp`). No preLaunch. |

### Root Orchestrator

| Profile | Type | Component | Port(s) |
|---|---|---|---|
| **Debug Root Manager** | Python | `root_orchestrator/system-manager-python` | 10000 / 50052 |
| **Debug Root Scheduler** | Go | `scheduler` (`ORCHESTRATION_PLANE=root`) | 10004 |
| **Debug Root Resource Abstractor** | Python | `resource-abstractor` | 11011 |
| **Debug JWT Generator** | Python | `root_orchestrator/jwt-generator` | 10011 |
| **Debug Root Addons Manager** | Python | `addons_engine/addons-manager` | 11101 |
| **Debug Root Addons Monitor** | Python | `addons_engine/addons-monitor` | — (docker.sock watcher) |
| **Debug Marketplace Manager** | Python | `addons_marketplace/marketplace-manager` | 11102 |

### Cluster Orchestrator

| Profile | Type | Component | Port(s) |
|---|---|---|---|
| **Debug Cluster Manager** | Python | `cluster_orchestrator/cluster-manager` | 10100 / 10101 |
| **Debug Cluster Scheduler** | Go | `scheduler` (`ORCHESTRATION_PLANE=cluster`) | 10105 |
| **Debug Cluster Resource Abstractor** | Python | `resource-abstractor` | 11012 |
| **Debug Cluster Addons Manager** | Python | `addons_engine/addons-manager` | 11201 |
| **Debug Cluster Addons Monitor** | Python | `addons_engine/addons-monitor` | — (docker.sock watcher) |

> The Resource Abstractor, Addons Manager, and Addons Monitor are single components deployed
> in **both** planes; the root/cluster profiles differ only by port and target MongoDB.

### Frontend / Storage

| Profile | Type | Component | Notes |
|---|---|---|---|
| **Debug Addons Dashboard** | Chrome | `addons_engine/addons-dashboard` | Runs `ng serve` on :4200 (via the background `serve-addons-dashboard` task) and attaches the browser debugger with source maps. One shared build for both planes. |
| **Debug CSI HostPath Driver** | Go | `csi/hostpath` | Standalone CSI driver (`--nodeid debug-node`); **not** part of the compose stack. Runs as root (creates `/var/lib/oakestra/csi` and binds its UNIX socket). No preLaunch. |

### Utility

| Profile | Type | Notes |
|---|---|---|
| **Cleanup Dev Environment** | — | Runs `cleanup-debug-env`: tears down both stacks and removes all containers + volumes. |

## Compounds

Launch several components together (dependencies first; `stopAll` stops the whole group):

| Compound | Launches |
|---|---|
| **Debug Full Root Orchestrator** | JWT Generator → Root Resource Abstractor → Root Scheduler → Root Manager |
| **Debug Full Cluster Orchestrator** | Cluster Resource Abstractor → Cluster Scheduler → Cluster Manager |
| **Debug Root Addons Stack** | Marketplace Manager → Root Addons Manager → Root Addons Monitor |
| **Debug Cluster Addons Stack** | Cluster Addons Manager → Cluster Addons Monitor |
| **Debug Full Stack (Root + Cluster)** | Root core + Cluster core (7 sessions) |

## Tasks reference

You normally don't run these directly — profiles invoke them — but they're available via
**Terminal → Run Task**:

- **Stack lifecycle:** `root-orchestrator-up` / `-down`, `cluster-orchestrator-up` / `-down`,
  `cleanup-debug-env`, `remove-containers`, `remove-volumes`.
- **`prep-debug-<component>-env`:** the full pre-launch sequence for one profile.
- **`remove-<container>`:** frees a single container's name/port.
- **`install-<component>-dependencies`:** installs one component's Python deps. For `uv`-managed
  services (`system-manager-python`, `cluster-manager`), dependencies and local shared libraries
  (`oakestra-utils`, `resource-abstractor-client`) are installed via `uv sync`. For components not
  yet migrated to `uv` (`resource-abstractor`, `jwt-generator`, `addons_*`), dependencies are
  installed from `requirements.txt`.
- **`serve-addons-dashboard`** (+ `install-addons-dashboard-dependencies`): background
  `ng serve` for the dashboard profile.

## Notes & gotchas

- **First launch is slow** — `*-up` builds images and pulls dependencies. Subsequent runs
  reuse them.
- **Compounds re-run shared preLaunch tasks** (e.g. `root-orchestrator-up`); `docker compose
  up` is idempotent, so this just adds a little startup time.
- **Harmless compound warning on open.** When you first open `launch.json`, VS Code may flag
  the compound `configurations` entries with *"Value is not accepted. Valid values: ."* /
  *"Please use unique configuration names."* This is a known VS Code bug
  ([microsoft/vscode#183712](https://github.com/microsoft/vscode/issues/183712)): the list of
  valid configuration names is populated asynchronously and is momentarily empty on load. The
  names are valid and the compounds run fine — the warning clears after any edit + save, or
  **Developer: Reload Window**.
- **Angular readiness pattern:** `serve-addons-dashboard` waits for Angular's esbuild
  "bundle generation complete" / "Local: http" line before attaching. If your Angular
  version prints a different ready message, update the task's `endsPattern`.
- **Shared libraries and UV workspace.** `system-manager-python`, `cluster-manager`, and the shared
  libraries under `libraries/` are managed via the root `uv` workspace. Running `uv sync` installs
  them into `.venv` in editable mode.
- **Local virtualenv isolation:** Docker container builds and CI workflows are fully isolated
  and unaffected. However, when running uv-managed and non-uv-managed modules locally and
  uncontainerized at the same time, sharing a single virtual environment can cause dependency
  conflicts, and `uv sync` will uninstall unmanaged packages. We recommend setting up secondary
  virtual environments in each non-uv module's subdirectory (e.g., `resource-abstractor/.venv`,
  `addons_engine/.venv`) when running them locally without Docker.
