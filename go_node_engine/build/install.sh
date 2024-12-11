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

# If arch is amd64, run ldconfig ../../wasmtime-go/target/x86_64-unknown-linux-gnu/release,
# else if arch is arm64, run ldconfig ../../wasmtime-go/target/aarch64-unknown-linux-gnu/release

if [ "$arch" == "amd64" ]; then
  sudo ldconfig ../../wasmtime-go/target/x86_64-unknown-linux-gnu/release
elif [ "$arch" == "arm64" ]; then
  sudo ldconfig ../../wasmtime-go/target/aarch64-unknown-linux-gnu/release
else
  echo "Invalid architecture"
  exit 1
fi


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
