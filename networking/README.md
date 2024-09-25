![net manager tests](https://github.com/oakestra/oakestra-net/actions/workflows/node_net_manager_tests.yml/badge.svg)
![net manager artifacts](https://github.com/oakestra/oakestra-net/actions/workflows/node-net-manager-artifacts.yml/badge.svg)
![root artifacts](https://github.com/oakestra/oakestra-net/actions/workflows/root_service_manager_image.yml/badge.svg)
![cluster artifacts](https://github.com/oakestra/oakestra-net/actions/workflows/cluster_service_manager_image.yml/badge.svg)

[![Stable](https://img.shields.io/badge/Latest%20Stable-%F0%9F%AA%97%20Accordion%20v0.4.301-green.svg)](https://github.com/oakestra/oakestra-net/tree/v0.4.301)
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

Then move the binary corresponding to your architecture to the current folder:
```
cp bin/<architecture>-NetManager .
```
> <architecture> is either arm-7 or amd64

Finally, install it using 
`./install.sh` 

## Run the NetManager

The Netmanager component is automatically managed by the NodeEngine. For a manual setup refer to: [node-net-manager/README.md](node-net-manager/README.md)







