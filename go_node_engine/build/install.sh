#!/usr/bin/env bash


# Get last argument from argument list 
arch="${@: -1}"

remote_host="localhost"
key=""
while getopts "hr:i:" flag; do
 case $flag in
   h) # Handle the -h flag
   echo "Usage: $0 <architecture>"
    echo "Supported architectures: amd64, arm64"
    echo "Options:"
    echo "  -h    Show this help message"
    echo "  -r [user@host]  Specify a remote user and host for NodeEngine installation"
    echo "  -i [key_path]   Specify the path to the SSH key for remote installation"
    exit 0
   ;;
   r) # Handle the -r flag
   echo "Remote host specified: $OPTARG"
   remote_host=$OPTARG
   ;;
   i) # set key [ath for remote host installation
   echo "Key path: $OPTARG"
   key=$OPTARG
   ;;
   \?)
   # Handle invalid options
   ;;
 esac
done

# Check if last argument is amd64 or arm64
if [ "$arch" != "amd64" ] && [ "$arch" != "arm64" ]; then
  echo "Invalid architecture specified. Supported architectures: amd64, arm64"
  echo "Usage: ./install.sh <architecture>"
  exit 1
fi


if [ "$remote_host" != "localhost" ]; then
  # Check if key is provided for remote installation
  opts=""
  if [ "$key" != "" ]; then
    echo "Using key $key for remote installation"
    opts="-i $key"
  fi
  # ssh to remote host and run the script
  echo "Moving NodeEngine_$arch, nodeengined_$arch, nodeengine.service and install.sh to remote host $remote_host"
  scp $opts NodeEngine_$arch nodeengined_$arch nodeengine.service ../nodeengine.service install.sh "$remote_host":~/
  # run node engine stop on remote host
  ssh $opts -t "$remote_host" "sudo NodeEngine stop >/dev/null 2>&1; ./install.sh $arch"
  exit $?
fi

systemd --version > /dev/null 2>&1
if [ ! $? -eq 0 ]; then
  /usr/lib/systemd/systemd --version > /dev/null 2>&1
  if [ ! $? -eq 0 ]; then
    echo "Systemd not present on this machine"
    exit 1
  fi
fi

#check containerd installation
if sudo systemctl status containerd.service &>/dev/null; then
  sudo systemctl daemon-reload
  sudo systemctl enable --now containerd
else
  wget https://github.com/containerd/containerd/releases/download/v2.0.0/containerd-2.0.0-linux-$arch.tar.gz
  chmod 777 containerd-2.0.0-linux-$arch.tar.gz
  sudo tar Cxzvf /usr/local containerd-2.0.0-linux-$arch.tar.gz
  wget https://raw.githubusercontent.com/containerd/containerd/main/containerd.service
  sudo mv containerd.service /lib/systemd/system/containerd.service
  wget https://github.com/opencontainers/runc/releases/download/v1.3.0/runc.$arch
  sudo install -m 755 runc.$arch /usr/local/sbin/runc
  wget https://github.com/containernetworking/plugins/releases/download/v1.7.1/cni-plugins-linux-$arch-v1.7.1.tgz
  sudo tar Cxzvf /opt/cni/bin cni-plugins-linux-$arch-v1.7.1.tgz
  sudo systemctl daemon-reload
  sudo systemctl enable --now containerd
  rm containerd-2.0.0-linux-$arch.tar.gz
  rm runc.$arch
  rm cni-plugins-linux-$arch-v1.7.1.tgz
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

if [ $? -eq 0 ]; then
  echo "Done, installation successful"
else
  echo "Installation failed, errors reported!"
  exit 1
fi
