echo "Usage ./start.sh <architecture>"
echo "supported architectures: amd64, arm-7"

export MQTT_BROKER_URL=192.168.42.186
export PUBLIC_WORKER_IP=192.168.43.221
export PUBLIC_WORKER_PORT=50103
export MQTT_BROKER_PORT=10003

sudo -E ./build/bin/$1-NetManager
