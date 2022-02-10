#!/bin/bash

# export CLUSTER_MANAGER_IP=<IP>
export CLUSTER_MANAGER_PORT=10000

pip3 install -r requirements.txt

# export FLASK_ENV=development
export FLASK_DEBUG=FALSE # TRUE for verbose logging #when True, MQTT logs twice because Flask opens second reloader thread

# TODO: remove before PR
export CLUSTER_MANAGER_IP=188.174.84.68
export CLUSTER_MANAGER_PORT=10000
export MQTT_BROKER_PORT=10003
export WORKER_PUBLIC_IP=188.174.84.68
export LAT=52.778016989191904
export LONG=8.073199727627696
export MY_PORT=3001
export REDIS_ADDR=redis://:workerRedis@localhost:6380
export VIVALDI_DIM=2
export GPS=TRUE

# Start node engine
echo "Start Node Engine#"
python3 app.py
