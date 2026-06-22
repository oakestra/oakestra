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
set -e

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
sudo mkdir -p /var/log/oakestra
sudo systemctl stop nodeengine 2>/dev/null || true
sudo systemctl stop netmanager 2>/dev/null || true

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

sudo systemctl daemon-reload

sudo chmod 755 /bin/NodeEngine
sudo chmod 755 /bin/nodeengined

if [ -e oakestra.logrotate ]; then
    logrotate_src="oakestra.logrotate"
elif [ -e ../oakestra.logrotate ]; then
    logrotate_src="../oakestra.logrotate"
else
    echo "Warning: oakestra.logrotate not found — log rotation will not be configured"
    logrotate_src=""
fi

if [ -n "$logrotate_src" ]; then
    sudo cp "$logrotate_src" /etc/logrotate.d/oakestra || echo "Warning: failed to install logrotate config — log rotation will not be configured"
fi

echo "Done, installation successful"
