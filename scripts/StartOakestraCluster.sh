#!/usr/bin/env bash

#Oakestra version?
if [ -z "$OAKESTRA_VERSION" ]; then
    OAKESTRA_VERSION='main'
fi

#Check if argument stop is passed, if yes, stop the cluster and exit
if [ "$1" == "stop" ]; then
    echo Stopping Oakestra Cluster Orchestrator...
    docker compose -f ~/.oakestra/cluster_orchestrator/cluster-orchestrator.yml down 
    exit 0
fi

echo 🌳 Running Oakestra Cluster 

# Function to check if OAKESTRA_VERSION is a tag (alpha-vX.Y.Z or vX.Y.Z)
is_tag() {
    if [[ "$1" =~ ^(alpha-)?v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        return 0
    else
        return 1
    fi
}

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
    current_os=$(uname)
    if [ "$current_os" = "Darwin" ]; then
        echo "Docker compose v2 or higher is required. Please refer to the official Docker documentation for installation instructions specific to your OS: https://docs.docker.com/compose/migrate/"
        exit 1
    else
        echo "Installing Docker Compose plugin"
        if [ ! -x "$(command -v apt-get)" ]; then
            sudo apt-get update
            sudo apt-get install docker-compose-plugin
        fi
        if [ ! -x "$(command -v yum)" ]; then
            sudo yum update
            sudo yum install docker-compose-plugin
        fi
    fi
fi

if [ -z "$cluster_location" ]; then

    # Installs jq if not present
    if [ ! -x "$(command -v jq)" ]; then
        echo "jq is not installed. Installing..."
        # Detect OS
        if [ "$(uname)" = "Darwin" ]; then
            # Install jq on macOS using Homebrew
            if ! command -v brew &> /dev/null; then
            echo "Homebrew is not installed. Please install Homebrew and re-run the script."
            exit 1
            fi
            brew install jq
        else
            # Install jq on Ubuntu/Debian based systems using apt
            sudo apt update && sudo apt install -y jq
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

rm -rf ~/.oakestra/cluster_orchestrator 2> /dev/null
mkdir -p ~/.oakestra/cluster_orchestrator 2> /dev/null

CURRENT_DIR=$(pwd)
cd ~/.oakestra/cluster_orchestrator 2> /dev/null

curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_VERSION/scripts/utils/downloadConfigFiles.sh > downloadConfigFiles.sh
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_VERSION/cluster_orchestrator/docker-compose.yml > cluster-orchestrator.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_VERSION/cluster_orchestrator/override-images-only.yml > override-cluster-images-only.yml

chmod +x downloadConfigFiles.sh
./downloadConfigFiles.sh cluster_orchestrator $OAKESTRA_VERSION

#If additional override files provided, download them
OAK_OVERRIDES=''

if [ ! -z "$OVERRIDE_FILES" ]; then
    IFS=, 
    # Split the string into an array using read -r -a
    for element in $OVERRIDE_FILES
    do
        echo "Download override: $element"
        wget -c https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_VERSION/cluster_orchestrator/$element 2> /dev/null
        OAK_OVERRIDES="${OAK_OVERRIDES}-f ${element} " 
    done
    IFS= 
    if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve the override."
        exit 1
    fi
fi

# Handle OAKESTRA_VERSION if set
BUILD_FLAG=''
COMPOSE_FILE='cluster-orchestrator.yml'
if [ ! -z "$OAKESTRA_VERSION" ]; then
    if is_tag "$OAKESTRA_VERSION"; then
        echo "🏷️  Using tag: $OAKESTRA_VERSION"
        # Update the override-cluster-images-only.yml with specific tag
        cp override-cluster-images-only.yml override-cluster-images-only.yml.bak
        sed "s/:latest/:$OAKESTRA_VERSION/g" override-cluster-images-only.yml.bak > override-cluster-images-only.yml
        rm override-cluster-images-only.yml.bak
        OAK_OVERRIDES="${OAK_OVERRIDES}-f override-cluster-images-only.yml "  
    else
        echo "🌿 Using branch: $OAKESTRA_VERSION"
        # if main branch, use latest images, if not main branch, build from source
        if [ "$OAKESTRA_VERSION" != "main" ]; then
            echo "🛠️ Non-main branch specified without a tag. Building from version."
            # Check remove / scripts from CURRENT_DIR if present to get to repo root
            cd $CURRENT_DIR
            if [[ "$CURRENT_DIR" == *"/scripts" ]]; then
                cd ..
            fi
            cd cluster_orchestrator
            echo "📦 Building images from source..."
            BUILD_FLAG=' --build'
            COMPOSE_FILE='docker-compose.yml'
        else
            OAK_OVERRIDES="${OAK_OVERRIDES}-f override-cluster-images-only.yml "
            echo "🌿 Using main branch, pulling latest images from Docker Hub."
        fi
    fi
fi

# If non-main branch and no override provided, update custom version of service manager to prevent potential issues with network policies in non-main branches
if [ "$OAKESTRA_VERSION" != "main" ]; then
    if [[ ! "$OVERRIDE_FILES" == *"override-no-network.yml"* ]] && [[ ! "$OVERRIDE_FILES" == *"override-custom-service-manager-version.yml"* ]]; then
        echo "🕸️ Setting network to latest alpha release"
        ALPHA_TAG=$(curl -s https://raw.githubusercontent.com/oakestra/oakestra-net/refs/heads/develop/version.txt)
        # Create override file with custom service manager version
        echo "services:
  root_service_manager:
    image: ghcr.io/oakestra/oakestra-net/root-service-manager:alpha-$ALPHA_TAG" > ~/.oakestra/root_orchestrator/override-custom-service-manager-version.yml
        OAK_OVERRIDES="${OAK_OVERRIDES}-f override-custom-service-manager-version.yml " 

        OAK_OVERRIDES="${OAK_OVERRIDES}-f ~/.oakestra/root_orchestrator/override-custom-service-manager-version.yml "
    fi
fi

if sudo docker ps -a | grep oakestra/cluster >/dev/null 2>&1; then
  echo 🚨 Oakestra cluster containers are already running. Please stop them before starting another cluster on this machine.
  echo 🪫 You can turn off the current cluster using: \$ docker compose -f ~/.oakestra/cluster_orchestrator/cluster-orchestrator.yml down
  exit 1
fi

command_exec="LIB_BRANCH=${OAKESTRA_VERSION} sudo -E docker compose -f ${COMPOSE_FILE} ${OAK_OVERRIDES} up ${BUILD_FLAG} -d"
echo executing "$command_exec"

eval "$command_exec"

echo 
echo 🌳 Oakestra Cluster Orchestrator is now starting up...
echo
echo 🖥️ Oakestra dashboard available at http://$SYSTEM_MANAGER_URL
echo 📊 Root Grafana dashboard available at http://$SYSTEM_MANAGER_URL:3000
echo 📊 Cluster Grafana dashboard available at http://$SYSTEM_MANAGER_URL:3001
echo 📈 You can access the APIs at http://$SYSTEM_MANAGER_URL:10000/api/docs
echo 🪫 You can turn off the cluster using: \$ docker compose -f ~/.oakestra/cluster_orchestrator/cluster-orchestrator.yml down
