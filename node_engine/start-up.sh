#!/bin/bash

#Requiring target architecture
if [ "$1" == "" ]; then
    echo "Please run the command specifying the current architecture"
    echo "Supported architectures: amd64, arm-7"
    echo "Example: ./start-up.sh amd64"
    exit 1
fi

# Run the netmanager in backgruond
sudo echo "Requiring SU"
cd app/NetManager/ && sudo CLUSTER_MANAGER_IP=$CLUSTER_MANAGER_IP CLUSTER_MANAGER_PORT=$CLUSTER_MANAGER_PORT ./bin/$1-NetManager &>> netmanager.log &
# Registering trap to kill the NetManager on exit
trap "ps -ax | grep NetManager | awk {'print $1'} | xargs sudo kill > /dev/null 2>&1" SIGINT SIGTERM EXIT
sleep 2

# create virtualenv
virtualenv --clear -p python3.8 .venv
virtualenv -p python3.8 .venv
source .venv/bin/activate
.venv/bin/pip install -r requirements.txt

# Install top level package in editable state (see setup.py)
export FLASK_ENV=development
export FLASK_DEBUG=FALSE # TRUE for verbose logging #when True, MQTT log twice because Flask opens second reloader thread

export CLUSTER_MANAGER_IP=188.174.53.20
export CLUSTER_MANAGER_PORT=10000
export MQTT_BROKER_PORT=10003
export WORKER_PUBLIC_IP=188.174.53.20
export LAT=52.778016989191904
export LONG=8.073199727627696
export MY_PORT=3001
export REDIS_ADDR=redis://:workerRedis@localhost:6380
export VIVALDI_DIM=2
export GPS=TRUE

# Start node engine
echo "Start Node Engine#"
.venv/bin/python app.py

