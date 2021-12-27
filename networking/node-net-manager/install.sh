echo "Usage ./install.sh <architecture>"
echo "supported architectures: amd64, arm-7"

if [ "$1" == "" ]; then
    echo "Architecture not set"
    extit 1
fi

cp ./build/bin/$1-NetManager /bin/NetManager