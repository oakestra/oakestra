# EdgeIO Net

This is the networking component that enables interactions between the microservices deployed in EdgeIO. 
The networking component resembles the multi-layer architecture of EdgeIO with the following components:

- Root service manager: register the cluster service manager and generates the subnetwork for each worker and cluster belonging to the infrastructure.
- Cluster service manager: this is the direct interface towards the nodes. This resolves the addresses required by each node. 
- NetManager: It's deployed on each node. It's responsible for the maintenance of a dynamic overlay network connecting the nodes.

This networking component creates a semantic addressing space where the IP addresses not only represent the final destination for a packet
but also enforces a balancing policy.

Please refer to our documentation.

## Semantic addressing (ServiceIPs)

A semantic address enforces a balancing policy towards all the instances of a service. 

- RR_IP (Currently implemented): IP address pointing every time to a random instance of a service. 
- Closest_IP (Under implementation): IP address pointing to the closest instance of a service.

Example: Given a service A with 2 instances A.a and A.b
- A has 2 ServiceIPs, a RR_IP and a Closest_IP. 
- A.a has an instance IP representing uniquely this instance.
- A.b has another instance IP representing uniquely this instance.
- If an instance of a service B uses the RR_IP of A, the traffic is balanced request after request toward A.a or A.b

The implementation happens at level 4, therefore as of now all the protocols absed on top of TCP and UDP are supported.

## Subnetworks

An overlay that spans seamlessly across the platform is only possible if each node has an internal sub-network that can be used to allocate an address for each newly deployed service. When a new node is attached to EdgeIO, a new subnetwork from the original addressing space is generated. All the services belonging to that node will have private namespace addresses belonging to that subnetwork.
As of now the network 172.16.0.0/12 represents the entire EdgeIO platform. From this base address each cluster contains subnetworks with a netmask of 26 bits that are assigned to the nodes. Each worker can then assign namespace ip addresses using the last 6 bits of the address. A namespace ip is yeat another address assigned to each instance only within the node boundaries. The address 172.30.0.0/16 is reserved to the ServiceIPs.
This network cut enables up to ≈ 15.360 worker nodes. Each worker can instantiate ≈ 62 containers, considering the address reserved internally for the networking components. 

## Packet proxying

The component that decides which is the recipient worker node for each packet is the ProxyTUN. This component is implemented as an L4 proxy which analyzes the incoming traffic, changes the source and destination address, and forwards it to the overlay network.
A packet approaching the proxy has a namespace IP as the source address and an IP belonging to the subnetwork of the Service and Instance IPs as a destination. 
The L4 packet also has a couple of source and destination ports used to maintain a connection and contact the correct application on both sides. The proxy’s job is to substitute the source and destination addresses according to the routing policy expressed by the destination address. 
The proxy converts the namespace address of the packet, belonging to the local network of the node, with the InstanceIP of that service’s instance.
This conversion enables the receiver to route the response back to the service instance deployed inside the sender’s node.
If the original destination address is an InstanceIP, the conversion is straightforward using the information available in the proxy’s cache. When the original destination address is a ServiceIP, the following four steps are executed:

- Fetch the routing policy
- Fetch the service instances
- Choose one instance  using the logic associated with the routing policy 
- Replace the ServiceIP with the namespace address of the resulting instance.

After the correct translation of source and destination addresses, the packet is encapsulated and sent to the tunnel only if the destination belongs to another node, or it is just sent back down to the bridge if the destination is in the same node.
