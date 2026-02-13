#!/bin/bash

#Oakestra version?
if [ -z "$OAKESTRA_VERSION" ]; then
    OAKESTRA_VERSION='main'
fi

#Check if argument stop is passed, if yes, stop the cluster and exit
if [ "$1" == "stop" ]; then
    echo Stopping Oakestra Root Orchestrator...
    docker compose -f ~/.oakestra/root_orchestrator/root-orchestrator.yml down 
    exit 0
fi

echo 🌳 Running Oakestra Root

# Function to check if OAKESTRA_VERSION is a tag (alpha-vX.Y.Z or vX.Y.Z)
is_tag() {
    if [[ "$1" =~ ^(alpha-)?v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        return 0
    else
        return 1
    fi
}

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

#Default configuration?
if [ -z "$SYSTEM_MANAGER_URL" ]; then
    echo 🔧 Using default configuration

    current_os=$(uname)
    
    # get IP address of this machine
    if [ "$current_os" = "Darwin" ]; then
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

rm -rf ~/.oakestra/root_orchestrator 2> /dev/null
mkdir -p ~/.oakestra/root_orchestrator 2> /dev/null

CURRENT_DIR=$(pwd)
cd ~/.oakestra/root_orchestrator 2> /dev/null

curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_VERSION/scripts/utils/downloadConfigFiles.sh > downloadConfigFiles.sh
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_VERSION/root_orchestrator/docker-compose.yml > root-orchestrator.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_VERSION/root_orchestrator/override-images-only.yml > override-root-images-only.yml

chmod +x downloadConfigFiles.sh
./downloadConfigFiles.sh run-a-cluster $OAKESTRA_VERSION

#If additional override files provided, download them
OAK_OVERRIDES=''

if [ ! -z "$OVERRIDE_FILES" ]; then
    IFS=, 
    # Split the string into an array using read -r -a
    for element in $OVERRIDE_FILES
    do
        echo "Download override: $element"
        wget -c https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_VERSION/root_orchestrator/$element 2> /dev/null
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
COMPOSE_FILE='root-orchestrator.yml'
if [ ! -z "$OAKESTRA_VERSION" ]; then
    if is_tag "$OAKESTRA_VERSION"; then
        echo "🏷️  Using tag: $OAKESTRA_VERSION"
        # Update the override-root-images-only.yml with specific tag
        cp override-root-images-only.yml override-root-images-only.yml.bak
        sed "s/:latest/:$OAKESTRA_VERSION/g" override-root-images-only.yml.bak > override-root-images-only.yml
        rm override-root-images-only.yml.bak
        OAK_OVERRIDES="${OAK_OVERRIDES}-f override-root-images-only.yml "  
    else
        echo "🌿 Using branch: $OAKESTRA_VERSION"
        # if main branch, use latest images, if not main branch, build from source
        if [ "$OAKESTRA_VERSION" != "main" ]; then
            echo "🛠️ Non-main branch specified without a tag. Building from version."
            git checkout $OAKESTRA_VERSION
            
            # Check remove / scripts from CURRENT_DIR if present to get to repo root
            cd $CURRENT_DIR
            if [[ "$CURRENT_DIR" == *"/scripts" ]]; then
                cd ..
            fi
            cd root_orchestrator
            echo "📦 Building images from source..."
            BUILD_FLAG=' --build'
            COMPOSE_FILE='docker-compose.yml'
        else
            OAK_OVERRIDES="${OAK_OVERRIDES}-f override-root-images-only.yml "
            echo "🌿 Using main branch, pulling latest images from Docker Hub."
        fi
    fi
fi

# If non-main branch and no override provided, update custom version of service manager to prevent potential issues with network policies in non-main branches
if [ "$OAKESTRA_VERSION" != "main" ]; then
    if [[ ! "$OVERRIDE_FILES" == *"override-no-network.yml"* ]] && [[ ! "$OVERRIDE_FILES" == *"override-custom-service-manager-version.yml"* ]]; then
        echo "🕸️ Setting network to latest alpha release"
        if is_tag "$OAKESTRA_VERSION"; then
            ALPHA_TAG=$(echo $OAKESTRA_VERSION | sed 's/alpha-//g')
        else
            ALPHA_TAG=$(curl -s https://raw.githubusercontent.com/oakestra/oakestra-net/refs/heads/develop/version.txt)
        fi
        # Create override file with custom service manager version
        echo "services:
  root_service_manager:
    image: ghcr.io/oakestra/oakestra-net/root-service-manager:alpha-$ALPHA_TAG" > ~/.oakestra/root_orchestrator/override-custom-service-manager-version.yml
        OAK_OVERRIDES="${OAK_OVERRIDES}-f override-custom-service-manager-version.yml " 

        OAK_OVERRIDES="${OAK_OVERRIDES}-f ~/.oakestra/root_orchestrator/override-custom-service-manager-version.yml "
    fi
fi

if sudo docker ps -a | grep oakestra/root >/dev/null 2>&1; then
  echo 🚨 Oakestra root containers are already running. Please stop them before starting the root orchestrator.
  echo 🪫 You can turn off the current root using: \$ docker compose -f ~/.oakestra/root_orchestrator/root-orchestrator.yml down
  exit 1
fi

command_exec="LIB_BRANCH=${OAKESTRA_VERSION} sudo -E docker compose -f ${COMPOSE_FILE} ${OAK_OVERRIDES} up ${BUILD_FLAG} -d"
echo executing "$command_exec"

eval "$command_exec"

echo 
echo 🌳 Oakestra Root Orchestrator is now starting up...
echo
echo 🖥️ Oakestra dashboard available at http://$SYSTEM_MANAGER_URL
echo 📊 Grafana dashboard available at http://$SYSTEM_MANAGER_URL:3000
echo 📈 You can access the APIs at http://$SYSTEM_MANAGER_URL:10000/api/docs
echo 🪫 You can turn off the cluster using: \$ docker compose -f ~/oakestra/root_orchestrator/root-orchestrator.yml down
