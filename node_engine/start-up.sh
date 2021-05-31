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
cd NetManager/ && sudo ./bin/$1-NetManager &>> netmanager.log &
# Registering trap to kill the NetManager on exit
trap "ps -ax | grep NetManager | awk {'print $1'} | xargs sudo kill > /dev/null 2>&1" SIGINT SIGTERM EXIT
sleep 2

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
