echo "Usage ./install.sh <architecture>"
echo "supported architectures: amd64, arm-7"

if [ "$1" == "" ]; then
    echo "Architecture not set"
    exit 1
fi

sudo cp ./build/bin/$1-NetManager /bin/NetManager
sudo mkdir /etc/netmanager
sudo cp ./config/tuncfg.json /etc/netmanager/tuncfg.json
sudo cp ./config/envcfg.json /etc/netmanager/envcfg.json
sudo cp ./config/netcfg.json /etc/netmanager/netcfg.json

sudo chmod 755 /bin/NetManager