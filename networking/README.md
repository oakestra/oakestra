![net manager tests](https://github.com/oakestra/oakestra-net/actions/workflows/node_net_manager_tests.yml/badge.svg)
![net manager artifacts](https://github.com/oakestra/oakestra-net/actions/workflows/node-net-manager-artifacts.yml/badge.svg)
![root artifacts](https://github.com/oakestra/oakestra-net/actions/workflows/root_service_manager_image.yml/badge.svg)
![cluster artifacts](https://github.com/oakestra/oakestra-net/actions/workflows/cluster_service_manager_image.yml/badge.svg)

[![Stable](https://img.shields.io/badge/Latest%20Stable-🎸Bass%20v0.4.400-green.svg)](https://github.com/oakestra/oakestra-net/tree/v0.4.400)
[![Github All Releases](https://img.shields.io/github/downloads/oakestra/oakestra-net/total.svg)]()

# Oakestra Net 🕸️🌳🕸️
This component enables the communication between services distributed across multiple [Oakestra](oakestra.io) nodes and clsuters.

This repository includes:

- **Net Manager**: The network daemon that needs to be installed on each Worker Node. This captures the services traffic, and creates the semantic overlay abstraction. See [Semantic Addressing](https://www.oakestra.io/docs/networking/semantic-addressing) for details.

- **Root/Cluster Service Managers**: Control plane components installed alongside Oakestra root and cluster orchestrators. They propagate and install the routes to the Net Manager components. 

>This networking component creates a semantic addressing space where the IP addresses not only represent the final destination for a packet
but also enforces a balancing policy.

## How to install the Net Manager daemon

### From official build

Follow the offical Oakestra [Get Started](https://github.com/oakestra/oakestra?tab=readme-ov-file#your-first-worker-node-🍃) guide to install the stable NetManager alongside oakestra worker node. 

### Build it on your own
Go inside the folder `node-net-manager/build` and run:
```
./build.sh
```

Finally, install it using 
`./install.sh <architecture>`

supported architectures are `arm64` or `amd64`.

## Run the NetManager

The Netmanager component is automatically managed by the NodeEngine. For a manual setup refer to: [node-net-manager/README.md](node-net-manager/README.md)


## How run the cluster service manager and root service manage components locally

You can:

1 - Navigate inside the corresponding cluster(root)-service-manager/service-manager folder

2 - Build the docker containers of the respective components using `docker build -t local_cluster_service_manager .` for the cluster service manager and `docker build -t local_root_service_manager .` for the root service manager. 

3 - Run your **standalone** oakestra root and cluster orchestrator as usual, but use the `override-local-service-manager.yml` to replace the official release images of these components with the new images you just built. 

E.g.: 
```
export OVERRIDE_FILES=override-local-service-manager.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/scripts/StartOakestraRoot.sh | sh - 
```






