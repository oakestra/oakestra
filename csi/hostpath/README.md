# Oakestra Hostpath CSI Plugin

A minimal, **headless** CSI driver that exposes an arbitrary host directory
inside a container via a Linux bind-mount. No CSI Controller service is
implemented; the plugin only handles the **Node** service RPCs.

---

## How it works

To enable this csi plugin in your worker node, follow these steps:

#### (1) Register the plugin to the node engine using:

`sudo NodeEngine config csi add csi.oakestra.io/hostpath unix:///var/lib/oakestra/csi/hostpath.sock`

#### (2) Run the plugin alongside the node engine. 

```
docker run -d \
  --name oakestra-hostpath-csi \
  --privileged \
  --pid host \
  -v /var/lib/oakestra/csi:/var/lib/oakestra/csi:rshared \
  -v /mnt/oakestra/hostpath:/mnt/oakestra/hostpath:rshared \
  ghcr.io/oakestra/oakestra/csi-hostpath-plugin:latest \
  -endpoint unix:///var/lib/oakestra/csi/hostpath.sock
```

#### (3) Start your node engine as usual


---

## Source path resolution

| Priority | Source |
|----------|--------|
| 1 | `volumes[].config.host_path` (explicit override in the `config` map) |
| 2 | `volumes[].volume_id` used directly as an absolute host path |

---

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--endpoint` | `unix:///var/lib/oakestra/csi/hostpath.sock` | Socket path the Node Engine connects to |
| `--nodeid` | `$(hostname)` | Node identifier returned by `NodeGetInfo` |

---

## Build and Run with Docker

The container needs `--privileged` (or at minimum `--cap-add SYS_ADMIN
--security-opt apparmor=unconfined`) and must share the host's mount namespace
so that bind-mounts made inside the container are visible to the Node Engine
and to containers it spawns.

```bash
# Build the image
docker build -t oakestra-hostpath-csi .

# Run
docker run -d \
  --name oakestra-hostpath-csi \
  --privileged \
  --pid host \
  -v /var/lib/oakestra/csi:/var/lib/oakestra/csi:rshared \
  -v /mnt/oakestra/hostpath:/mnt/oakestra/hostpath:rshared \
  oakestra-hostpath-csi \
  -endpoint unix:///var/lib/oakestra/csi/hostpath.sock
```

> [!IMPORTANT]
> The `:rshared` mount propagation flag is **critical**. Without it, bind-mounts created by the CSI plugin inside the container will not be visible to the host or to application containers, causing volumes to appear empty.

> [!TIP]
> Bind-mount every host directory the plugin will expose into the container
> (with the same path) so the plugin can `stat` and `mount` them.

---

## How does it work inside the Node Engine

> N.b. The driver advertises **no** `STAGE_UNSTAGE_VOLUME` capability, so the Node
Engine skips `NodeStageVolume`/`NodeUnstageVolume` entirely — only
`NodePublishVolume` and `NodeUnpublishVolume` are called.

Add the plugin to the Oakestra Node Engine configuration file (`config.json`):

The Node Engine will probe the plugin at startup via `Probe` →
`GetPluginInfo` → `NodeGetCapabilities` and register it if all three calls
succeed.

---

## Deployment descriptor example

```json
{
  "sla_version": "v2.0",
  "customerID": "Admin",
  "applications": [
    {
      "application_name": "myapp",
      "application_namespace": "default",
      "application_desc": "App with hostpath volume",
      "microservices": [
        {
          "microservice_name": "worker",
          "microservice_namespace": "default",
          "virtualization": "container",
          "code": "docker.io/library/alpine:latest",
          "memory": 128,
          "vcpus": 1,
          "volumes": [
            {
              "volume_id":   "/host/data",
              "csi_driver":  "csi.oakestra.io/hostpath",
              "mount_path":  "/data"
            }
          ]
        }
      ]
    }
  ]
}
```

To override the host path via `config` (useful when `volume_id` is a logical
name rather than a path):

```json
"volumes": [
  {
    "volume_id":   "my-named-volume",
    "csi_driver":  "csi.oakestra.io/hostpath",
    "mount_path":  "/data",
    "config": {
      "host_path": "/srv/storage/my-named-volume"
    }
  }
]
```

---

## CSI RPCs implemented

| Service | RPC | Status |
|---------|-----|--------|
| Identity | `GetPluginInfo` | ✅ implemented |
| Identity | `GetPluginCapabilities` | ✅ implemented (empty – headless) |
| Identity | `Probe` | ✅ implemented |
| Node | `NodePublishVolume` | ✅ implemented (bind-mount) |
| Node | `NodeUnpublishVolume` | ✅ implemented (unmount + cleanup) |
| Node | `NodeGetCapabilities` | ✅ implemented (empty – no staging) |
| Node | `NodeGetInfo` | ✅ implemented |
| Node | `NodeGetVolumeStats` | ✅ implemented (statfs) |
| Node | `NodeStageVolume` | ❌ Unimplemented (not advertised) |
| Node | `NodeUnstageVolume` | ❌ Unimplemented (not advertised) |
| Node | `NodeExpandVolume` | ❌ Unimplemented |
| Controller | *(all)* | ❌ Not registered (headless) |
