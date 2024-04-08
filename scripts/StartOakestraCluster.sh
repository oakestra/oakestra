#!/usr/bin/env bash
echo 🌳 Running Oakestra Cluster 

#Oakestra branch?
if [ -z "$OAKESTRA_BRANCH" ]; then
    OAKESTRA_BRANCH='main'
fi

if [ ! -z "$CLUSTER_LOCATION" ]; then
    cluster_location=$CLUSTER_LOCATION
fi

if [ ! -z "$CLUSTER_NAME" ]; then
    cluster_name=$CLUSTER_NAME
fi

if [ -z "$cluster_location" ]; then
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

    cluster_location=$(echo $latitude,$longitude,1000)
    export CLUSTER_LOCATION=$cluster_location
fi

echo Leave a field empty to keep the current value
echo "Enter Cluster Name (current: $cluster_name): " 
read cluster_name_input < /dev/tty
echo "Enter Cluster Location (current: $cluster_location): "
read cluster_location_input < /dev/tty
echo "Enter Root Orchestrator URL (current: $SYSTEM_MANAGER_URL): "
read system_manager_url_input < /dev/tty

if [ ! -z "$cluster_name_input" ]; then
    echo 🛠️ Setting new cluster name 
    export CLUSTER_NAME=$cluster_name_input
fi

if [ ! -z "$cluster_location_input" ]; then
    echo 🛠️ Setting new cluster location 
    export CLUSTER_LOCATION=$cluster_location_input
fi

if [ ! -z "$system_manager_url_input" ]; then
    echo 🛠️ Setting new root url 
    export SYSTEM_MANAGER_URL=$system_manager_url_input
fi

if [ -z "$CLUSTER_NAME" ]; then
    echo ❌❌❌ Cluster Name is required
    exit 1
fi

if [ -z "$CLUSTER_LOCATION" ]; then
     echo ❌❌❌ Cluster Location is required
    exit 1
fi

if [ -z "$SYSTEM_MANAGER_URL" ]; then
     echo ❌❌❌ Root Orchestrator URL is required
    exit 1
fi

rm -rf ~/oakestra 2> /dev/null
mkdir ~/oakestra 2> /dev/null

cd ~/oakestra 2> /dev/null

wget -c https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/cluster-orchestrator.yml 2> /dev/null

mkdir prometheus 2> /dev/null
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/prometheus/prometheus.yml > prometheus/prometheus.yaml

mkdir mosquitto 2> /dev/null
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/mosquitto/mosquitto.conf > mosquitto/mosquitto.conf


#If additional override files provided, download them
OAK_OVERRIDES=''

if [ ! -z "$OVERRIDE_FILES" ]; then
    IFS=, 
    # Split the string into an array using read -r -a
    for element in $OVERRIDE_FILES
    do
        echo "Download override: $element"
        wget -c https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/$element 2> /dev/null
        OAK_OVERRIDES="${OAK_OVERRIDES}-f ${element} " 
    done
    IFS= 
    if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve the override."
        exit 1
    fi
fi
command_exec="sudo -E docker compose -f cluster-orchestrator.yml ${OAK_OVERRIDES}up"
echo executing "$command_exec"

eval "$command_exec"