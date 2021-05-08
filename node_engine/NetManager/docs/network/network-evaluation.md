# Evaluation of EdgeIO networking capabilities 

## Aspects that can be tested 

* Troughput
	* Example: Toward Highly Scalable Load Balancing in Kubernetes Clusters [3], **Evaluation And discussion section**
* Balancing  
	* Balancing of the cpu load
		* Example: The Design of Multi-Metric Load Balancer for Kubernetes [4], **experiment and results section**
* Bandwidth 
	* Example: Networking Analysis and Performance Comparison of Kubernetes CNI Plugins [5],  **Test Results section**
* Fault tolerance
	* Example: Scalability evaluation of VPN technologies for secure container networking [6], **section IV and V**   

## Setup 

* Service to Service communication 
	* Same node
	* Different nodes
	* Different clusters (if possible)
* Client to Service communication
	* Direct communication
	* Rerouted communication
* Service to Client 
* Services communication during migration
* Services communication during scaling up 

## Simulating the network

The network setups that we can physically manage are not many. In order to have a wide range of tested environments, as part of the network development, several scenarios must be tested. The use of network simulators is highly recommended. 

A list of configurable networking simulators is available in the article Hogie et al. **An Overview of MANETs Simulation** [1].
This article, even if it's quite old, can be a good starting point with the due precautions. In fact for instance the network simulator ns-2 is presented even if now exists ns-3. 


A comparison of a few networking simulators (NS-2, NS-3, and OMNet) in terms of architecture and performance has been done in  **Evaluating network test scenarios for network simulators systems** [2]

![Net simulator timeline](https://journals.sagepub.com/na101/home/literatum/publisher/sage/journals/content/dsna/2017/dsna_13_10/1550147717738216/20171026/images/large/10.1177_1550147717738216-fig1.jpeg)

Suggestion: Let's start with our own networking environment and let's use a network simulator for more complex environment later on.

## References

[1]  Hogie L, Bouvry P and Guinand F. An overview of MANETs simulation. Electron Notes Theor Comput Sci 2006; 150: 81–101 

[2] Anis Zarrad and Izzat Alsmadi. Evaluating network test scenarios for network simulators systems. International Journal of Distributed Sensor Networks
2017, Vol. 13(10)

[3] Nguyen Dinh Nguyen and Taehong Kim. Toward Highly Scalable Load Balancing in Kubernetes Clusters. IEEE Communications Magazine (Volume: 58, Issue: 7, July 2020)

[4] Qingyang Liu, Haihong E, Meina Song. The Design of Multi-Metric Load Balancer for Kubernetes. 2020 International Conference on Inventive Computation Technologies (ICICT)

[5] Ritik Kumar, Munesh Chandra Trivedi. Networking Analysis and Performance Comparison of Kubernetes CNI Plugins. Part of the Advances in Intelligent Systems and Computing book series (AISC, volume 1158)

[6] Tom Goethals, Dwight Kerkhove, Bruno Volckaert, Filip De Turck. Scalability evaluation of VPN technologies for secure container networking. 15th International Conference on Network and Service Management (CNSM 2019)




