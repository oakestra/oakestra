echo "Insert username used to log in on all the remote machines:"
read username

while :
do

echo "################################################"
echo "#                                              #"
echo "#          EdgeIO Deployment Helper            #"
echo "#                                        v0.1  #"
echo "################################################"
echo "################################################"
echo "#                                              #"
echo "#  1: Configure nodes with interactive iface   #"
echo "#  2: Configure nodes with wcfg.yaml           #"
echo "#  3: Avoid sudo password for current user     #"
echo "#  4: Prepare all the nodes                    #"
echo "#  5: Deploy EdgeIO with current config        #"
echo "#  6: Undeploy with current configuration      #"
echo "#  7: Restart all the worker nodes             #"
echo "#  8: Stop only the worker nodes               #"
echo "#                                              #"
echo "################################################"

read choice

if [[ choice -eq 1 ]]; then
 echo "### 1: Configure nodes with interactive iface ###"
 cd support 
 ./genconfig.sh
 cd ../
fi
if [[ choice -eq 2 ]]; then
 echo "### 2: Configure nodes with wcfg.yaml ###"
 cd support
 pip3 install --ignore-installed PyYAML
 python3 read_from_yaml.py 
 cd ../
fi
if [[ choice -eq 3 ]]; then
 echo "### 3: Avoid sudo password for current user  ###"
 cd support
 ./config_workers_sudo.sh $username
 cd ../
fi
if [[ choice -eq 4 ]]; then
 echo "### 4: Prepare all the nodes                 ###"
 cd support
 ./prepare_workers.sh
 cd ../
fi
if [[ choice -eq 5 ]]; then
 echo "### 5: Deploy EdgeIO with current config     ###"
 cd support
 ./deployment.sh $username
 cd ../
fi
if [[ choice -eq 6 ]]; then
 echo "### 6: Undeploy with current configuration   ###"
 cd support
 ./undeployment.sh $username
 cd ../
fi
if [[ choice -eq 7 ]]; then
 echo "### 7: Restart all the worker nodes         ###"
 cd support
 ./restart_workers_cluster.sh $username
 cd ../
fi
if [[ choice -eq 8 ]];then
 echo "### 8: Stopping workers                   ###"
 cd support
 ./stop_workers.sh $username
 cd ../
fi

echo "################################################"
echo "################################################"
echo " "
echo " "
done

