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
cd NetManager/ && sudo CLUSTER_MANAGER_IP=$CLUSTER_MANAGER_IP CLUSTER_MANAGER_PORT=$CLUSTER_MANAGER_PORT ./bin/$1-NetManager &>> netmanager.log &
# Registering trap to kill the NetManager on exit
trap "ps -ax | grep NetManager | awk {'print $1'} | xargs sudo kill > /dev/null 2>&1" SIGINT SIGTERM EXIT
sleep 2

# create virtualenv
#virtualenv --clear -p python3.8 .venv
virtualenv -p python3.8 .venv

source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

# export FLASK_ENV=development
export FLASK_DEBUG=FALSE # TRUE for verbose logging #when True, MQTT logs twice because Flask opens second reloader thread

#export CLUSTER_MANAGER_IP=118.195.253.88
export CLUSTER_MANAGER_IP=$MYIP
export CLUSTER_MANAGER_PORT=10000
export WORKER_PUBLIC_IP=$MYIP
export SYSTEM_MANAGER_IP=$MYIP
export SYSTEM_MANAGER_PORT=10000
export LAT=48.19349
export LONG=11.63067
export MY_PORT=3001
export REDIS_ADDR=redis://:workerRedis@localhost:6380
export MONITORING_PORT=4000

# Run monitoring component in background (&) and redirect stdout+stderr to log file (overwrite content: %>)
echo "Start monitoring component"
.venv/bin/python Monitoring/app.py &> Monitoring/monitoring.log &
#.venv/bin/celery -A Monitoring.monitoring.celeryapp worker --detach --logfile=Monitoring/celery.log --loglevel=DEBUG --concurrency=1

# Start node engine
echo "Start node engine"
.venv/bin/python node_engine.py
