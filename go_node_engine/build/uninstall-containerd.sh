#!/bin/bash

sudo systemctl stop containerd.service
sudo systemctl disable containerd.service
sudo rm /lib/systemd/system/containerd.service
sudo systemctl daemon-reload
sudo rm /usr/local/bin/containerd 
sudo rm /usr/local/bin/ctr
sudo rm /usr/bin/containerd
sudo rm /usr/bin/ctr
sudo rm /bin/containerd
sudo rm /bin/ctr