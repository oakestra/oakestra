#!/bin/bash
echo 🌳 Running Oakestra Root \n

#Oakestra branch?
if [-z "$OAKESTRA_BRANCH" ]; then
    OAKESTRA_BRANCH='main'
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

rm -rf ~/oakestra 2> /dev/null
mkdir ~/oakestra 2> /dev/null

cd ~/oakestra 2> /dev/null

wget -c https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/root-orchestrator.yml 2> /dev/null

mkdir prometheus 2> /dev/null
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/prometheus/prometheus.yml > prometheus/prometheus.yaml

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
command_exec="sudo -E docker compose -f root-orchestrator.yml ${OAK_OVERRIDES}up"
echo executing "$command_exec"

eval "$command_exec"