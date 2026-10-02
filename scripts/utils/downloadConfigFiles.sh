#!/bin/bash

set -u

repo_folder=${1:?"repository folder is required"}
repo_branch=${2:?"repository branch or tag is required"}

metrics_disabled() {
    case ",${OVERRIDE_FILES:-}," in
        *override-no-observe*) return 0 ;;
        *) return 1 ;;
    esac
}

check_metrics_compatibility() {
    if metrics_disabled; then
        return 0
    fi

    if [ "$(uname -s)" != "Linux" ]; then
        echo "Error: the host metrics stack requires Linux. Use override-no-observe.yml on unsupported hosts."
        return 1
    fi

    local docker_version docker_major
    docker_version=$(sudo docker version --format '{{.Server.Version}}' 2>/dev/null) || {
        echo "Error: unable to read the Docker Engine server version."
        return 1
    }
    docker_major=${docker_version%%.*}
    docker_major=${docker_major#v}

    if ! [[ "$docker_major" =~ ^[0-9]+$ ]] || [ "$docker_major" -lt 25 ]; then
        echo "Error: the metrics stack requires Docker Engine 25 or newer (found ${docker_version:-unknown})."
        echo "Use override-no-observe.yml to run Oakestra without the observability stack."
        return 1
    fi
}

download_file() {
    local config_file=$1
    local destination=$config_file
    local temporary="${destination}.tmp.$$"
    local url="https://raw.githubusercontent.com/oakestra/oakestra/${repo_branch}/${repo_folder}/${config_file}"

    mkdir -p "$(dirname "$destination")"
    if ! curl -fsSL "$url" -o "$temporary"; then
        rm -f "$temporary"
        echo "Error: failed to download $url"
        return 1
    fi
    mv "$temporary" "$destination"
}

common_config_files="config/grafana-dashboards.yml config/grafana-datasources.yml config/loki.yml config/config.alloy config/alerts/grafana-rules.yml config/alerts/grafana-contact-point.yml config/dashboards/logs-dashboard.json config/dashboards/log-statistics-dashboard.json config/dashboards/resources-dashboard.json"

case "$repo_folder" in
    root_orchestrator)
        config_files="prometheus/prometheus.yml $common_config_files"
        ;;
    cluster_orchestrator)
        config_files="prometheus/prometheus.yml prometheus/prometheus-host.yml mosquitto/mosquitto.conf $common_config_files"
        ;;
    run-a-cluster)
        config_files="prometheus/prometheus.yml prometheus/prometheus-root.yml mosquitto/mosquitto.conf $common_config_files"
        ;;
    *)
        echo "Error: unsupported configuration folder: $repo_folder"
        exit 1
        ;;
esac

check_metrics_compatibility || exit 1

# Remove the retired combined logs and statistics dashboard.
rm -f config/dashboards/dashboard.json

for config_file in $config_files; do
    download_file "$config_file" || exit 1
done
