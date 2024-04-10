mkdir -p config/alerts 2> /dev/null
mkdir -p config/dashbaords 2> /dev/null
mkdir prometheus 2> /dev/null
mkdir mosquitto 2> /dev/null

repo_folder=$1

curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/prometheus/prometheus.yml > prometheus/prometheus.yaml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/mosquitto/mosquitto.conf > mosquitto/mosquitto.conf
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/config/grafana-dashboards.yml > config/grafana-dashboards.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/config/grafana-datasources.yml > config/grafana-datasources.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/config/loki.yml > config/loki.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/config/promtail.yml > config/promtail.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/config/alerts/rules.yml > config/alerts/rules.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/$OAKESTRA_BRANCH/$repo_folder/config/dashboards/dashboard.json > config/dashboards/dashboard.json
