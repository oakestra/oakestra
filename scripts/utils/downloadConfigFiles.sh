#!/bin/bash

# Create directories silently
mk_dir() {
  local dir_path="$1"
  mkdir -p "$dir_path" 2> /dev/null
}

# Repository folder as first argument
repo_folder="$1"

# "1DOC" for 1DOC configuration, "root" for root orchestrator, "cluster" for cluster orchestrator
layer="$2"

# Base URL for downloads
base_url="https://raw.githubusercontent.com/TheDarkPyotr/oakestra/$OAKESTRA_BRANCH/$repo_folder"

# Configuration files for 1DOC
DOC_config_files=(
  #"mosquitto/mosquitto.conf:mosquitto/mosquitto.conf"
  "prometheus/prometheus.yml:prometheus/prometheus.yml"
  "config/grafana-dashboards.yml:config/grafana-dashboards.yml"
  "config/grafana-datasources.yml:config/grafana-datasources.yml"
  "config/loki.yml:config/loki.yml"
  "config/promtail.yml:config/promtail.yml"
  "config/alerts/rules.yml:config/alerts/rules.yml"
  "config/dashboards/dashboard.json:config/dashboards/dashboard.json"
)

# Configuration files for root (excluding mosquitto.conf)
root_config_files=(
  "prometheus/prometheus.yml:prometheus/prometheus.yml"
  "config/grafana-dashboards.yml:config/grafana-dashboards.yml"
  "config/grafana-datasources.yml:config/grafana-datasources.yml"
  "config/loki.yml:config/loki.yml"
  "config/promtail.yml:config/promtail.yml"
  "config/alerts/rules.yml:config/alerts/rules.yml"
  "config/dashboards/dashboard.json:config/dashboards/dashboard.json"
)

cluster_config_files=(
  #"mosquitto/mosquitto.conf:mosquitto/mosquitto.conf"
  "prometheus/prometheus.yml:prometheus/prometheus.yml"
  "config/grafana-dashboards.yml:config/grafana-dashboards.yml"
  "config/grafana-datasources.yml:config/grafana-datasources.yml"
  "config/loki.yml:config/loki.yml"
  "config/promtail.yml:config/promtail.yml"
  "config/alerts/rules.yml:config/alerts/rules.yml"
  "config/dashboards/dashboard.json:config/dashboards/dashboard.json"
)

# Create directories
mk_dir "prometheus"
#mk_dir "mosquitto"
mkdir "mosquitto" 2> /dev/null
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/mosquitto/mosquitto.conf > mosquitto/mosquitto.conf
mk_dir "config/alerts"
mk_dir "config/dashboards"

# Download configuration files based on layer
case "$layer" in
  "1DOC")
    config_files=("${DOC_config_files[@]}")
    ;;
  "root")
    config_files=("${root_config_files[@]}")
    ;;
  "cluster")
    config_files=("${cluster_config_files[@]}")
    ;;
  *)
    echo "Error: Invalid layer '$layer'. Valid options are '1DOC','root' or 'cluster'."
    exit 1
    ;;
esac

# Download configuration files one by one
for file_spec in "${config_files[@]}"; do
   source_url="${base_url}/${file_spec%%:}"  
   dest_path="${file_spec##*:}"
  curl -sfL "$source_url" > "$dest_path"
  echo "Downloaded $source_url to $dest_path"
done

echo "Configuration files downloaded successfully!"
