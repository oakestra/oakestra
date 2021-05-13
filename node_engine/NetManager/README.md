# NetManager
This component enables the communication between services distributed across multiple nodes.

The Network manager is divided in 4 main components: 

* Environment Manager: Creates the Host Bridge, is responsible for the creation and destruction of network namespaces, and for the maintenance of the Translation Table used by the other components. 
* ProxyTunnel: This is the communication channel. This component enables the service to service communication within the platform. In order to enable the communication the translation table must be kept up to date, otherwise this component asks the Environment manager for the "table query" resolution process. Refer to the official documentation for more details. 
* mDNS: used for .local name resolution. Refer to the documentation for details.
* API: used to trigger a new deployment, the management operations on top of the already deployed services and to receive information about the services. 

# Structure

```

.
├── docs/
│			Description:
│				Documentation and current proposals 
├── bin/
│			Description:
│				Binary executable compiled files 
├── config/
│			Description:
│				Configuration files used by the environment maanger and the proxyTunnel. These configuration files are used only 
│               for testing purpose to create a local environment without the need of plugging the compennt to the local orchestrator. 
├── env/
│			Description:
│				The environment manager implementation resides here. 
├── proxy/
│			Description:
│				This is where the ProxyTunnel implmentation belongs
├── testEnvironment/
│			Description:
│				Executable files that can be used to test the Netowrk Manager locally. 
├── api/
│			Description:
│				This is where the exposed api is implemented
├── setup.sh
│			Description:
│				Used to install al the dependencies
└──  NetManager.go
			Description:
				Entry point to startup the NetworkManager

```

# Installation

## Development setup
The development setup can be used to test locally the tunneling mechanism without the use of the Cluster orchestrator. This setup requires 2 different machines namely Host1 and Host2.
* go 1.12+ required 
* run the setup.sh to install the dependencies on each machine 

### Host1
use: `sudo go run testEnvironment/TwoNetnsDev1.go`
when prompted insert the *Host2* IP address used to resolve the tunneling.

This script will create the local subnetwork `172.19.1.0/24` with 2 network namespaces deployed `myapp1` and `myapp2`

You can now access these namespaces with the ip utility and run inside them whatever you prefer.
example:

`sudo ip netns exec myapp1 ip a s`

This command will show the current interfaces inside this namespace and the current ip address that should be `172.19.1.12`.

### Host2
use: `sudo go run testEnvironment/TwoNetnsDev2.go`
when prompted insert the *Host1* IP address used to resolve the tunneling.

This script will create the local subnetwork `172.19.2.0/24` with 2 network namespaces deployed `myapp1` and `myapp2`

You can now access these namespaces with the ip utility and run inside them whatever you prefer.
example:

`sudo ip netns exec myapp1 ip a s`

This command will show the current interfaces inside this namespace and the current ip address that should be `172.19.2.12`.

### Test the setup

Now try running a sample hello world flask app inside Host1/myapp1. Let's suppose we have our app.py file and that this application exposes the port 50001. 

Run the flask app on Host1:
`sudo netns exec myapp1 python3 app.py`

Now on Host2 try reaching the app deployed on the Host1/myapp1
`sudo netns exec myapp2 curl 172.19.1.12`

Now let's try from Host2 to load balance between all the myapp2 instances across Host1 and Host2 using the proxy address. In this example setup the proxy address 172.30.0.0 will map to myapp1.
`sudo netns exec myapp1 curl 172.30.0.0`

If there is nothing deployed behind the myapp2 namespace you probably will only get a connection refused error. That error is sent by the linux kernel but this means that you actually reached the namesapce correctly. 

### Subnetworks
With this default test configuration the Subnetwork hierarchy is:

###Container Network:
`172.16.0.0/12`

This is the network where all the IP addresses belongs

###Service IP subnetwork:
`172.32.0.0/16`

This is a special subnetwork where all the VirtualIP addresses belongs. An address belonging to this range must be
translated to an actual container address and pass trough the proxy. 

###Bridge Subnetwork:
`172.19.1.0/24`

Address where all the containers of this node belong. Each new container will have an address from this space.

###Prohibited port numbers
Right now a deployed service can't use the same port as the proxy tunnel


## Deployment
Note, most of the following must still be implemented

### With binary files

Execute the binary files directly specifying the Cluster address. This will trigger the registration procedure. 
`sudo ./bin/Networkmanager -cluster <ip-address>`

### With go commandline

* go 1.12+ required
* run the setup.sh to install the dependencies on each machine

Execute the Network manager with 
`sudo go run NetManager.go -cluster <ip-address>`
