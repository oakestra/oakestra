#!/bin/bash

config_files="prometheus/prometheus.yml mosquitto/mosquitto.conf config/grafana-dashboards.yml config/grafana-datasources.yml config/loki.yml config/promtail.yml config/alerts/rules.yml config/dashboards/dashboard.json"
repo_folder=$1
repo_branch=$2

mkdir -p config/alerts 2> /dev/null
mkdir -p config/dashboards 2> /dev/null
mkdir prometheus 2> /dev/null
mkdir mosquitto 2> /dev/null

for config_file in ${config_files}; do
    rm $config_file 2> /dev/null
    touch $config_file 
    curl -sL https://raw.githubusercontent.com/oakestra/oakestra/$repo_branch/$repo_folder/$config_file -o $config_file
done
