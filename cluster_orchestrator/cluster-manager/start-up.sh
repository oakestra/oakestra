#!/bin/bash

# docker-compose --file ../docker-compose-amd64.yml up -d

# create virtualenv
virtualenv --clear -p python3 .venv
source .venv/bin/activate

BRANCH=${BRANCH:-$(git rev-parse --abbrev-ref HEAD)}
.venv/bin/pip install -r <(sed "s|\${BRANCH}|${BRANCH}|g" cluster_orchestrator/cluster-manager/requirements.txt)

export FLASK_ENV=development
export FLASK_DEBUG=True # TRUE for logging

export MQTT_BROKER_URL=localhost
export MQTT_BROKER_PORT=10003

export CLUSTER_MONGO_URL=localhost
export CLUSTER_MONGO_PORT=10107

export SYSTEM_MANAGER_URL=localhost
export SYSTEM_MANAGER_PORT=10000

export CLUSTER_SCHEDULER_URL=localhost
export CLUSTER_SCHUEDLER_PORT=10105

export CLUSTER_SERVICE_MANAGER_ADDR=localhost
export CLUSTER_SERVICE_MANAGER_PORT=10110

export SYSTEM_MANAGER_GRPC_PORT=50052

# get public IP
PUBLIC_IP=$(curl -sLf "https://api.ipify.org")
if [ $? -ne 0 ]; then
    echo "Error: Failed to retrieve your public IP address."
    exit 1
fi
# get geo coordinates of public IP
ipLocation=$(curl -sLf "https://ipinfo.io/$PUBLIC_IP/json")
if [ $? -ne 0 ]; then
    echo "Error: Failed to retrieve your public IP address."
    exit 1
fi
# Extract latitude and longitude
latitude=$(echo "$ipLocation" | jq -r '.loc | split(",") | .[0]')
longitude=$(echo "$ipLocation" | jq -r '.loc | split(",") | .[1]')
echo Default cluster location $(echo $latitude,$longitude,1000)

export CLUSTER_LOCATION=$(echo $latitude,$longitude,1000)
export CLUSTER_NAME=cluster_local

export MY_PORT=8000

.venv/bin/python cluster_manager.py
