#!/bin/bash

if [ "$CLUSTER_MANAGER_IP" == "" ]; then
    echo "CLUSTER_MANAGER_IP NOT SET!"
    echo "Please run: export CLUSTER_MANAGER_IP=<cluster_orchestrator_ip> "
    exit 1
fi

if [ "$CLUSTER_MANAGER_PORT" == "" ]; then
    echo "CLUSTER_MANAGER_PORT NOT SET!"
    echo "Please run export CLUSTER_MANAGER_IP=<cluster_orchestrator_ip> to set a custom one"
    echo "switching to default 10000 port"
    export CLUSTER_MANAGER_PORT=10000
fi

if [ "$MQTT_BROKER_URL" == "" ]; then
    echo "MQTT_BROKER_URL NOT SET!"
    echo "Please run export MQTT_BROKER_URL=<ip or url of the mqtt broker> to set a custom one"
    echo "switching to default $CLUSTER_MANAGER_IP"
    export MQTT_BROKER_URL=$CLUSTER_MANAGER_IP
fi

if [ "$MQTT_BROKER_PORT" == "" ]; then
    echo "MQTT_BROKER_PORT NOT SET!"
    echo "switching to default 10003 port"
    export MQTT_BROKER_PORT=10003
fi

if [ "$PUBLIC_WORKER_IP" == "" ]; then
    echo "PUBLIC_WORKER_IP NOT SET!"
    echo "Please run export PUBLIC_WORKER_IP=<ip or hostname>"
    exit 1
fi

if [ "$PUBLIC_WORKER_PORT" == "" ]; then
    echo "PUBLIC_WORKER_PORT NOT SET!"
    echo "switching to default 50103 port"
    export PUBLIC_WORKER_PORT=50103
fi

if [ "$LAT" == "" ]; then
    echo "WORKER LATITUDE NOT SET!"
    echo "switching to mock GPS mode"
    export GPS=TRUE
fi

if [ "$LONG" == "" ]; then
    echo "WORKER LONGITUDE NOT SET!"
    echo "switching to mock GPS mode"
    export GPS=TRUE
fi

if [ "$GPS" == "" ]; then
    echo "WORKER GPS MODE NOT SET!"
    echo "switching to mock GPS mode"
    export GPS=TRUE
fi

if [ "$VIVALDI_DIM" == "" ]; then
    echo "WORKER VIVALDI COORDINTE DIMENSION NOT SET!"
    echo "switching to default"
    export VIVALDI_DIM=2
fi

if [ "$REDIS_ADDR" == "" ]; then
    echo "WORKER REDIS ADDR NOT SET!"
    echo "switching to default"
    export REDIS_ADDR=redis://:workerRedis@localhost:6380
fi

pip3 install -r requirements.txt

# export FLASK_ENV=development
export FLASK_DEBUG=FALSE # TRUE for verbose logging #when True, MQTT logs twice because Flask opens second reloader thread


# Start node engine
echo "Start Node Engine#"
.venv/bin/python3 app.py
