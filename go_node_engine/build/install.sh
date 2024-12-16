#!/usr/bin/env bash

if [ -z "$1" ]; then
    echo "Architecture not set"
    echo "Usage ./install.sh <architecture>"
    echo "supported architectures: amd64, arm64"
    exit 1
fi

systemd --version > /dev/null 2>&1
if [ ! $? -eq 0 ]; then
  /usr/lib/systemd/systemd --version > /dev/null 2>&1
  if [ ! $? -eq 0 ]; then
    echo "Systemd not present on this machine"
    exit 1
  fi
fi

arch="$1"

echo "Downloading wasmtime library"
# Download wasmtime-go library to /usr/local/lib/wasmtime-go for dynamic linking (depending on the architecture)
if [ "$arch" == "amd64" ]; then
  wget -lv https://github.com/bytecodealliance/wasmtime/releases/download/v25.0.0/wasmtime-v25.0.0-x86_64-linux-c-api.tar.xz -O /tmp/wasmtime-c-api.tar.xz
elif [ "$arch" == "arm64" ]; then
  wget -lv https://github.com/bytecodealliance/wasmtime/releases/download/v25.0.0/wasmtime-v25.0.0-aarch64-linux-c-api.tar.xz -O /tmp/wasmtime-c-api.tar.xz
else
  echo "Architecture not supported"
  exit 1
fi

echo "Extracting wasmtime library"
tar -xf /tmp/wasmtime-c-api.tar.xz -C /tmp
rm /tmp/wasmtime-c-api.tar.xz

echo "Copying wasmtime library to /usr/local/lib"
sudo mkdir -p /usr/local/lib/wasmtime-go

# Copy the content of wasmtime library that starts with name wasmtime-* to /usr/local/lib/wasmtime-go
sudo cp -r /tmp/wasmtime-*/* /usr/local/lib/wasmtime-go
rm -rf /tmp/wasmtime-*

echo "Wasmtime library copied to /usr/local/lib/wasmtime-go"

#check containerd installation
if sudo systemctl | grep -Fq 'containerd'; then
  sudo systemctl daemon-reload
  sudo systemctl enable --now containerd
else
  wget https://github.com/containerd/containerd/releases/download/v1.6.1/cri-containerd-cni-1.6.1-linux-$arch.tar.gz
  chmod 777 cri-containerd-cni-1.6.1-linux-$arch.tar.gz
  sudo tar --no-overwrite-dir -C / -xzf cri-containerd-cni-1.6.1-linux-$arch.tar.gz
  sudo systemctl daemon-reload
  sudo systemctl enable --now containerd
  rm cri-containerd-cni-1.6.1-linux-$arch.tar.*
fi

#install latest version
sudo mkdir /var/log/oakestra >/dev/null 2>&1
sudo systemctl stop nodeengine >/dev/null 2>&1
sudo systemctl stop netmanager >/dev/null 2>&1

#compatibility with build script
if [ -e NodeEngine ]
then
    mv NodeEngine NodeEngine_$1
fi
if [ -e nodeengined ]
then
    mv nodeengined nodeengined_$1
fi

sudo cp NodeEngine_$1 /bin/NodeEngine
sudo cp nodeengined_$1 /bin/nodeengined

if [ -e nodeengine.service ]
then
    sudo cp nodeengine.service /etc/systemd/system/nodeengine.service
else
    sudo cp ../nodeengine.service /etc/systemd/system/nodeengine.service
fi  

sudo systemctl daemon-reload >/dev/null 2>&1

sudo chmod 755 /bin/NodeEngine
sudo chmod 755 /bin/nodeengined

[ $? -eq 0 ] && echo "Done, installation successful" || echo "Installation failed, errors reported!"
