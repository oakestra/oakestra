#!/bin/bash

# export CLUSTER_MANAGER_IP=<IP>
export CLUSTER_MANAGER_PORT=10000
export CLUSTER_MANAGER_IP=188.174.87.76
export MQTT_BROKER_PORT=10003
export WORKER_PUBLIC_IP=188.174.87.76
export LAT=52.778016989191904
export LONG=8.073199727627696
export MY_PORT=3001
export REDIS_ADDR=redis://:workerRedis@localhost:6380
export VIVALDI_DIM=2
export GPS=TRUE

pip3 install -r requirements.txt

# export FLASK_ENV=development
export FLASK_DEBUG=FALSE # TRUE for verbose logging #when True, MQTT logs twice because Flask opens second reloader thread


# Start node engine
echo "Start Node Engine#"
.venv/bin/python3 app.py
