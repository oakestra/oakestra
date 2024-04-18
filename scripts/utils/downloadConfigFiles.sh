#!/bin/bash

config_files="prometheus/prometheus.yml mosquitto/mosquitto.conf config/grafana-dashboards.yml config/grafana-datasources.yml config/loki.yml config/promtail.yml config/alerts/rules.yml config/dashboards/dashboard.json"
repo_folder=$1
repo_branch=$2

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

for config_file in ${config_files}; do
    rm $config_file 2> /dev/null
    touch $config_file 
    curl -sL https://raw.githubusercontent.com/oakestra/oakestra/$repo_branch/$repo_folder/$config_file -o $config_file

