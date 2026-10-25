# NodeEngine 

The node engine is the core of the Oakestra worker node. 

## Build the NodeEngine 

Move to the build directory and run `./build.sh` and then `./install.sh $(dpkg --print-architecture)`.

## Use the NodeEngine 

You can start the node engine using the `sudo NodeEngine` command.

**N.B.** If you wish to start the NodeEngine standalone, without network component ([NetManager](https://github.com/oakestra/oakestra-net/tree/develop/node-net-manager)), first run `sudo NodeEngine config network off` to shut down overlay network mode.

`NodeEngine -h` gives you the list of available startup options.

```
Start a New Oakestra Worker Node

Usage:
  NodeEngine [flags]
  NodeEngine [command]

Available Commands:
  config      configure the node engine
  help        Help about any command
  logs        tail check node engine logs
  status      check status of node engine
  stop        stops the NodeEngine (and NetManager if configured)
  version     Print the version number of NodeEngine

Flags:
  -c, --certFile string         Path to certificate for TLS support
  -a, --clusterAddr string      Address of the cluster orchestrator without port (default "localhost")
  -p, --clusterPort int         Port of the cluster orchestrator (default 10100)
  -d, --detatch                 Run the NodeEngine in the background (daemon mode)
      --flops-learner           Enables the ML-data-server sidecar for data collection for FLOps learners.
  -h, --help                    help for NodeEngine
      --image-builder           Checks if the host has QEMU (apt's qemu-user-static) installed for building multi-platform images.
  -k, --keyFile string          Path to key for TLS support
  -l, --logs string             Directory for application's logs (default "/tmp")
  -o, --overlayNetwork string   Options: default,disabled,custom:<path>. <path> points to the overlay component socket. (default "default")

Use "NodeEngine [command] --help" for more information about a command.
```

## Requirements

NodeEngine uses the containerd 2.x Go client, so the containerd daemon on the worker should be 2.x as well. CI tests against 2.3. containerd 1.7 still works on a best effort basis, but it went EOL on 2026-09-30. 1.6 and older are not supported.

`oak install worker` from [oakestra-cli](https://github.com/oakestra/oakestra-cli) installs containerd 2.x if it's missing, and the NVIDIA Container Toolkit on GPU machines. Without the CLI, `build/install.sh` installs containerd 2.x and runc when no containerd is running.

## GPU support

NodeEngine passes NVIDIA GPUs into containers via [CDI](https://github.com/cncf-tags/container-device-interface). The worker needs the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html) and a CDI spec:
```bash
sudo nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml
nvidia-ctk cdi list   # should list nvidia.com/gpu=0, nvidia.com/gpu=all, ...
```
`oak install worker` does both. Rerun `cdi generate` after a driver upgrade or GPU change, otherwise the spec points at stale libraries.

## How to configure additional OCI runtimes

Containerd (the default NodeEngine container runtime) allows any OCI-compliant runtime to be used as a plugin. This means that you can use any OCI-compliant runtime with Oakestra, including:
- runc
- runsc 
- runu 

and many more. The default container runtime is runc.

To configure a new OCI runtime, you need to edit the containerd configuration file. The default location for this file is `/etc/containerd/config.toml`.

### Example configuration file for runsc secure containerd runtime (aka gVisor)

1. Add the following configuration in your containerd configuration file `/etc/containerd/config.toml`:
```toml
version = 3

[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runsc]
  runtime_type = "io.containerd.runsc.v1"
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runsc.options]
  TypeUrl = "io.containerd.runsc.v1.options"
  ConfigPath = "/etc/containerd/runsc.toml"
```

Older 1.x configs (`version = 2` or no version) still work, containerd 2.x migrates them on startup.

2. Install gVisor on your system as usual following the [gVisor installation instructions](https://gvisor.dev/docs/user_guide/install/).

3. Restart the Oakestra node engine. You will notice how the new runtime is automatically detected and available for your applications.  

