#!/bin/bash
echo 🌳 Running Oakestra Root \n

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
fi

mkdir ~/oakestra 2> /dev/null

cd ~/oakestra 2> /dev/null

wget -c https://raw.githubusercontent.com/oakestra/oakestra/develop/run-a-cluster/root-orchestrator.yml 2> /dev/null

mkdir prometheus 2> /dev/null
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/run-a-cluster/prometheus/prometheus.yml > prometheus/prometheus.yaml

sudo -E docker compose -f root-orchestrator.yml $OAK_OVERRIDES up