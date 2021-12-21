echo "Usage ./start.sh <architecture>"
echo "supported architectures: amd64, arm-7"

export MQTT_BROKER_URL=192.168.42.186
export MQTT_BROKER_PORT=10003

sudo -E ./build/bin/$1-NetManager