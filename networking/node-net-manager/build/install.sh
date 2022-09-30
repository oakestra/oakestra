echo "Usage ./install.sh <architecture>"
echo "supported architectures: amd64, arm-7"

sudo cp ./NetManager /bin/NetManager
sudo mkdir /etc/netmanager
sudo cp tuncfg.json /etc/netmanager/tuncfg.json
sudo cp netcfg.json /etc/netmanager/netcfg.json

sudo chmod 755 /bin/NetManager