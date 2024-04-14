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

if [ -z "$cluster_location" ]; then

    # Installs jq if not present
    if [ ! -x "$(command -v jq)" ]; then
        echo "jq is not installed. Installing..."
        # Detect OS
        if [ "$(uname)" == "Darwin" ]; then
            # Install jq on macOS using Homebrew
            if ! command -v brew &> /dev/null; then
            echo "Homebrew is not installed. Please install Homebrew and re-run the script."
            exit 1
            fi
            brew install jq
        else
            # Install jq on Ubuntu/Debian based systems using apt
            sudo apt update && sudo apt install jq
        fi
        echo "jq installation complete."
    else
        echo "jq is already installed."
    fi
    if [ $? -ne 0 ]; then
        echo "Error: Failed to install jq. Please install it manually."
        exit 1
    fi

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

ls -al

rm -rf ~/oakestra 2> /dev/null
mkdir ~/oakestra 2> /dev/null

cd ~/oakestra 2> /dev/null

curl -sfL https://raw.githubusercontent.com/TheDarkPyotr/oakestra/$OAKESTRA_BRANCH/run-a-cluster/cluster-orchestrator.yml > cluster-orchestrator.yml

curl -sfL https://raw.githubusercontent.com/TheDarkPyotr/oakestra/$OAKESTRA_BRANCH/scripts/utils/downloadConfigFiles.sh > downloadConfigFiles.sh


chmod +x downloadConfigFiles.sh
./downloadConfigFiles.sh run-a-cluster cluster

   if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve your public IP address."
        exit 1
    fi
#If additional override files provided, download them
OAK_OVERRIDES=''

if [ ! -z "$OVERRIDE_FILES" ]; then
    IFS=, 
    # Split the string into an array using read -r -a
    for element in $OVERRIDE_FILES
    do
        echo "Download override: $element"
        wget -c https://raw.githubusercontent.com/TheDarkPyotr/oakestra/$OAKESTRA_BRANCH/run-a-cluster/$element 2> /dev/null
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