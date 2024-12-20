if [ -z "$1" ]; then
    echo "Architecture not set"
    echo "Usage ./install.sh <architecture>"
    echo "supported architectures: amd64, arm64"
    exit 1
fi

if [ -e NetManager ]
then
    sudo cp ./NetManager /bin/NetManager
else
    sudo cp bin/$1-NetManager /bin/NetManager
fi

sudo mkdir /etc/netmanager >/dev/null 2>&1
sudo mkdir /var/log/oakestra >/dev/null 2>&1

if [ ! -e /etc/netmanager/netcfg.json ]
then
    if [ -e tuncfg.json ]
    then
        sudo cp tuncfg.json /etc/netmanager/tuncfg.json
        sudo cp netcfg.json /etc/netmanager/netcfg.json
    else
        sudo cp ../config/tuncfg.json /etc/netmanager/tuncfg.json
        sudo cp ../config/netcfg.json /etc/netmanager/netcfg.json
    fi
fi

sudo systemctl stop nodeengine >/dev/null 2>&1
sudo systemctl stop netmanager >/dev/null 2>&1

if [ -e netmanager.service ]
then
    sudo cp netmanager.service /etc/systemd/system/netmanager.service
else
    sudo cp ../netmanager.service /etc/systemd/system/netmanager.service
fi  

sudo systemctl daemon-reload >/dev/null 2>&1

sudo chmod 755 /bin/NetManager