#!/bin/bash
echo 🌳 Running Oakestra 1-DOC 

#Oakestra branch?
if [ -z "$OAKESTRA_BRANCH" ]; then
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

# Detect OS
current_os=$(uname)

# Installs jq if not present
if [ ! -x "$(command -v jq)" ]; then
  echo "jq is not installed. Installing..."
  if [ $current_os = "Darwin" ]; then
    # Install jq on macOS using Homebrew
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

#Default configuration?
if [ "$2" != "custom" ]; then
    echo 🔧 Using default configuration

    # if custom IP not set use default one
    if [ -z "$SYSTEM_MANAGER_URL" ]; then
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

rm -rf ~/oakestra 2> /dev/null
mkdir ~/oakestra 2> /dev/null

cd ~/oakestra 

curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/scripts/utils/downloadConfigFiles.sh > downloadConfigFiles.sh
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/1-DOC.yaml > 1-DOC.yaml

chmod +x downloadConfigFiles.sh
./downloadConfigFiles.sh run-a-cluster $OAKESTRA_BRANCH

if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve config files"
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
        curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/run-a-cluster/$element > $element
        OAK_OVERRIDES="${OAK_OVERRIDES}-f ${element} " 
    done
    IFS= 
    if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve the override."
        exit 1
    fi
fi

if sudo docker ps -a | grep oakestra >/dev/null 2>&1; then
  echo 🚨 Oakestra containers are already running. Please stop them before starting a new 1-DOC cluster.
  echo 🪫 You can turn off the current cluster using: \$ docker compose -f ~/oakestra/1-DOC.yaml down
  exit 1
fi

command_exec="sudo -E docker compose -f 1-DOC.yaml ${OAK_OVERRIDES}up -d"
echo executing "$command_exec"

eval "$command_exec"

echo 
echo 🌳 Oakestra 1-DOC is now starting up...
echo
echo 🖥️ Oakestra dashboard available at http://$SYSTEM_MANAGER_URL
echo 📊 Grafana dashboard available at http://$SYSTEM_MANAGER_URL:3000
echo 📈 You can access the APIs at http://$SYSTEM_MANAGER_URL:10000/api/docs
echo 🪫 You can turn off the cluster using: \$ docker compose -f ~/oakestra/1-DOC.yaml down
