if [ "$1" == "" ]; then
    echo "Architecture not set"
    echo "Usage ./install.sh <architecture>"
    echo "supported architectures: amd64, arm-7"
    exit 1
fi

systemd --version > /dev/null 2>&1
if [ ! $? -eq 0 ]; then
  echo "Systemd not present on this machine"
  exit 1
fi

#check containerd installation
if sudo systemctl | grep -Fq 'containerd'; then
  sudo systemctl daemon-reload
  sudo systemctl enable --now containerd
else
  wget https://github.com/containerd/containerd/releases/download/v1.6.1/cri-containerd-cni-1.6.1-linux-amd64.tar.gz
  chmod 777 cri-containerd-cni-1.6.1-linux-amd64.tar.gz
  sudo tar --no-overwrite-dir -C / -xzf cri-containerd-cni-1.6.1-linux-amd64.tar.gz
  sudo systemctl daemon-reload
  sudo systemctl enable --now containerd
  rm cri-containerd-cni-1.6.1-linux-amd64.tar.*
fi

#install latest version
sudo cp bin/NodeEngine_$1 /bin/NodeEngine
sudo chmod 755 /bin/NodeEngine

[ $? -eq 0 ] && echo "Done, installation successful" || echo "Installation failed, errors reported!"
