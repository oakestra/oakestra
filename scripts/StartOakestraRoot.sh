#!/bin/bash
echo 🌳 Running Oakestra Root \n

#Oakestra branch?
if [-z "$OAKESTRA_BRANCH" ]; then
    OAKESTRA_BRANCH='main'
fi

# Check if docker and docker compose installed 
if [ ! -x "$(command -v docker)" ]; then
  echo "Docker is not installed. Please refer to the official Docker documentation for installation instructions specific to your OS: https://docs.docker.com/engine/install/"
  exit 1
fi
echo Checking docker compose version
sudo docker compose version
if [ $? -ne 0 ]; then
    echo "Docker compose v2 or higher is required. Please refer to the official Docker documentation for installation instructions specific to your OS: https://docs.docker.com/compose/migrate/"
    exit 1
fi

#Default configuration?
if [ -z "$SYSTEM_MANAGER_URL" ]; then
    echo 🔧 Using default configuration
    
    # get IP address of this machine
    # get IP address of this machine
    if [ $current_os = "Darwin" ]; then
        export SYSTEM_MANAGER_URL=$(ipconfig getifaddr en0)
    else
        export SYSTEM_MANAGER_URL=$(ip route get 1.1.1.1 | grep -oP 'src \K\S+')
    fi
    if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve interface IP."
        exit 1
    fi
    echo Default node IP: $SYSTEM_MANAGER_URL
fi

rm -rf ~/oakestra 2> /dev/null
mkdir ~/oakestra 2> /dev/null

cd ~/oakestra 2> /dev/null

curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/root-orchestrator.yml > root-orchestrator.yml

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