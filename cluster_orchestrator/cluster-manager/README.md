# Cluster Manager

The Cluster Manager is a component in the Cluster Orchestrator. Edge nodes register themselves to the Cluster Manager to be part of a cluster/group of compute nodes and to be "schedulable"

## Purpose of Cluster Manager

- maintains the Cluster Database (mongoDB) by inserting / removing worker nodes, and inserting/updating/removing tasks 
- provides endpoints to start/stop a container inside its cluster. Cluster Scheduler is asked to calculate exact placement
- aggregates data of nodes and sends via HTTP to Root Orchestrator (information channel)
- provides `/init` endpoint (which is upgraded to websocket) and handles initialization process with client


## Incoming Endpoints which can be used e.g. by the System Manager

- /api/init to register worker nodes
- /api/deploy to ask the Cluster Scheduler for a job placement and to contact the target worker node
- /api/delete to delete a job in its cluster
- /api/move to migrate jobs from a node to another node within the cluster
- /api/replicate to scale up or down the number of microservices within the cluster


## Outgoing Endpoints to other components

- Cluster Manager registers at System Manager  (Websocket register phase)
- Cluster Manager asks the Cluster Scheduler for placement calculations
- Cluster Manager reports aggregated information to the System Manager
- Cluster Manager pulls edge node information from MQTT Broker

## Start the Cluster Manager

Please start the Cluster Manager with `./start-up.sh`.
A virtualenv will be started and cluster-manager will start up.

Use the docker-compose file to start other necessary Cluster Orchestrator components (Mqtt Broker + MongoDB + Redis as Cluster Scheduler-Queue)

## IP Geolocation
To be able to perform IP geolocation, the cluster orchestrator requires a local GeoLite2 database for IP geolocation lookup.
Since the database file is too large to upload it to the git repository, the cluster operator has to download the .csv file
and place in under /cluster-manager/geolocation
Download: https://dev.maxmind.com/geoip/geolite2-free-geolocation-data?lang=en
## Built With

- Python3.8.5
  - Flask
  - Flask-MQTT
  - bson
  - Flask-PyMongo
  - Flask-SocketIO
  - eventlet
  - requests
  - pyyaml
  - APScheduler


## Lessons Learnt

- Logging was disabled because SocketIO uses eventlet wsgi as async_mode and the default log of eventlet is stderr. Thus it was necessary to override the log parameter of eventlet. Flask uses the default logging library (https://docs.python.org/3.8/library/logging.html#module-logging). See Constructor of eventlet wsgi here: http://eventlet.net/doc/modules/wsgi.html?highlight=wsgi . Furthermode, use `eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)` as server startup.
- Race Condition in Websocket Connection with System_Manager: time.sleep(1) before sending the first Websocket answer so that it receives the connection establishment before sending on the negotiated namespace. See comment in code. Cluster Manager gets first message from System Manager and then realizes, the connection is established. Therefore, wait after receiving the first message until connection establishment is realized and send then the answer to the System Manager. The same applies for the Websocket communication between Node Engine and Cluster Manager.
