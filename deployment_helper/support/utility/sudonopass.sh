res=$(cat /etc/sudoers.d/edgeio  | grep -c $1)

if [[ $res -gt 0 ]]; then
        echo "already set up"
	exit
fi

echo "no sudo setuo found for current user"
echo "adding current user to /etc/sudoers.d/edgeio"
sudo -S sh -c "echo '$1   ALL=(ALL:ALL) NOPASSWD:ALL' >> /etc/sudoers.d/edgeio"
sudo -S sh -c "echo '$1   ALL=(ALL:ALL) NOPASSWD:ALL' >> /etc/sudoers.d/groupsettings"
#sudo -S sh -c "mv /etc/sudoers.d/groupsettings /etc/sudoers.d/groupsettings.old"
echo "SUCCESS!! :)"
