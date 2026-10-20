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
# 2.x is what we support, 1.7 still works but is best effort, anything older is unsupported
CONTAINERD_VERSION="2.3.7"
RUNC_VERSION="1.5.2"

if sudo systemctl | grep -Fq 'containerd'; then
  installed_version=$(containerd --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n 1 || true)
  case "$installed_version" in
    0.*|1.[0-6].*)
      echo "Warning: containerd $installed_version is older than 1.7 and not supported, please upgrade to containerd 2.x" ;;
    1.*)
      echo "Note: containerd $installed_version is only supported on a best effort basis (1.7 reached EOL on 2026-09-30), consider upgrading to containerd 2.x" ;;
  esac
  sudo systemctl daemon-reload
  sudo systemctl enable --now containerd
else
  # The release tarball only has the binaries, so the unit file and runc are fetched separately.
  # We skip the CNI plugins, NetManager takes care of networking.
  wget https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-$arch.tar.gz
  sudo tar -C /usr/local -xzf containerd-${CONTAINERD_VERSION}-linux-$arch.tar.gz
  sudo mkdir -p /usr/local/lib/systemd/system
  sudo wget -O /usr/local/lib/systemd/system/containerd.service https://raw.githubusercontent.com/containerd/containerd/v${CONTAINERD_VERSION}/containerd.service
  wget https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.$arch
  sudo install -m 755 runc.$arch /usr/local/sbin/runc
  sudo systemctl daemon-reload
  sudo systemctl enable --now containerd
  rm containerd-${CONTAINERD_VERSION}-linux-$arch.tar.gz runc.$arch
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
