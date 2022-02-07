#!/bin/bash

# export CLUSTER_MANAGER_IP=<IP>
export CLUSTER_MANAGER_PORT=10000

pip3 install -r requirements.txt

# export FLASK_ENV=development
export FLASK_DEBUG=FALSE # TRUE for verbose logging #when True, MQTT logs twice because Flask opens second reloader thread

python3 node_engine.py
