#!/bin/bash



# `$exec_mode` identify the folder of the config files to download based on the components we are running (1DOC, root or cluster)
# Specifically:
# - `1DOC` ($exec_mode=`run-a-cluster`)
# - `root_orchestrator` ($exec_mode=`root_orchestrator`)
# - `cluster_orchestrator` ($exec_mode=`cluster_orchestrator`)



# Define base URL

exec_mode=$1 #determine if deploy 1DOC, root or cluster component
repo_folder=$2 #determine the folder, at moment static to `run-a-cluster`
base_url="https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder"

mkdir -p config/alerts 2> /dev/null
mkdir -p config/dashboards 2> /dev/null
mkdir prometheus 2> /dev/null

# 1DOC and cluster orchestrator require mosquitto broke
config_1DOC=(
  "prometheus/prometheus.yml"
  "mosquitto/mosquitto.conf"
  "config/grafana-dashboards.yml"
  "config/grafana-datasources.yml"
  "config/loki.yml"
  "config/promtail.yml"
  "config/alerts/rules.yml"
  "config/dashboards/dashboard.json"
)

# Root orchestrator does not require mosquitto broker
config_root=(
  "prometheus/prometheus.yml"
  "config/grafana-dashboards.yml"
  "config/grafana-datasources.yml"
  "config/loki.yml"
  "config/promtail.yml"
  "config/alerts/rules.yml"
  "config/dashboards/root-dashboard.json"
)

config_cluster=(
  "prometheus/prometheus.yml"
  "config/grafana-dashboards.yml"
  "config/grafana-datasources.yml"
  "config/loki.yml"
  "config/promtail.yml"
  "config/alerts/rules.yml"
  "config/dashboards/cluster-dashboard.json"
)


#$exec mode determine if mosquitto broker is required or not
# root deployment does not require mosquitto
case "$exec_mode" in
  "1DOC")
    config_files=("${config_1DOC[@]}")
    mkdir mosquitto 2> /dev/null
    ;;
  "root")
    config_files=("${config_root[@]}")
    ;;
  "cluster")
    config_files=("${config_cluster[@]}")
    mkdir mosquitto 2> /dev/null

    ;;
  *)
    echo "Error: Invalid layer '$exec_mode'. Valid options are 'run-a-cluster','root_orchestrator' or 'cluster_orchestrator'."
    exit 1
    ;;
esac

# Download files with a loop


for file in "${config_files[@]}"; do
  echo "Downloading $base_url/$file into $file"
  curl -sfL "$base_url/$file" > "$file"
done

# Success message
echo "Downloaded files from branch $OAKESTRA_BRANCH repository folder '$exec_mode'."
