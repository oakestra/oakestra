#!/bin/bash

# create virtualenv
virtualenv --clear -p python3.8 .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

# export FLASK_ENV=development
export FLASK_DEBUG=FALSE # TRUE for verbose logging #when True, MQTT logs twice because Flask opens second reloader thread

export CLUSTER_MANAGER_IP=131.159.24.210
# export CLUSTER_MANAGER_IP=localhost
export CLUSTER_MANAGER_PORT=10000

.venv/bin/python node_engine.py
