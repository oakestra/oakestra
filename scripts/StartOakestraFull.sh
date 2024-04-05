#!/bin/bash
echo 🌳 Running Oakestra 1-DOC 

#Alpha version required?
if [ "$1" = "alpha" ]; then
    OAK_OVERRIDES="-f override-alpha-versions.yaml"
fi

#Default configuration?
if [ "$2" != "custom" ]; then
    echo 🔧 Using default configuration
    
    # get IP address of this machine
    export SYSTEM_MANAGER_URL=$(ip route get 1.1.1.1 | grep -oP 'src \K\S+')
    if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve interface IP."
        exit 1
    fi
    echo Default node IP: $SYSTEM_MANAGER_URL

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
    export CLUSTER_NAME=default_cluster
fi

mkdir ~/oakestra 2> /dev/null

cd ~/oakestra 2> /dev/null

wget -c https://raw.githubusercontent.com/oakestra/oakestra/develop/run-a-cluster/1-DOC.yaml 2> /dev/null

mkdir prometheus 2> /dev/null
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/run-a-cluster/prometheus/prometheus.yml > prometheus/prometheus.yaml

mkdir mosquitto 2> /dev/null
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/run-a-cluster/mosquitto/mosquitto.conf > mosquitto/mosquitto.conf

sudo -E docker compose -f 1-DOC.yaml $OAK_OVERRIDES up