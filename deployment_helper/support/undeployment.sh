hostname_to_ip()
{
  	while IFS=, read localhostname localip; do
        	if [[ $hostname == $localhostname ]]; then
			ip=$localip
			break
		fi
	done < node.txt
}

echo "\n"
echo "#####################"
echo "### Root shutdown ###"
echo "#####################"

rootlist=""
for root in $(cat root.txt); do
        rootlist=$rootlist""$root" "
done

hostname=""
rootip=""
for root in $rootlist; do
        hostname=$root
        hostname_to_ip
        echo "Executing docker-compose down on node: <$hostname,$ip>"
	ssh $1@$ip "cd EdgeIO_deployment/root/root_orchestrator/; docker-compose down"
	ssh $1@$ip "cd EdgeIO_deployment/root/root_orchestrator/; docker-compose rm"
	rootip=$ip
done

echo "\n"
echo "#####################"
echo "##Clusters shutdown #"
echo "#####################"

clusterlist=""
for cluster in $(cat cluster.txt); do
        clusterlist=$clusterlist""$cluster" "
done

hostname=""
clusternum=1
for cluster in $clusterlist; do
        hostname=$cluster
        hostname_to_ip
        echo "Executing docker-compose down on cluster$clusternum node: <$hostname,$ip>"
        ssh $1@$ip "cd EdgeIO_deployment/cluster$clusternum/cluster_orchestrator/;export SYSTEM_MANAGER_URL='$rootip'; export CLUSTER_NAME='cluster$clusternum'; export CLUSTER_LOCATION='cluster$clusternum'; docker-compose down"
	ssh $1@$ip "cd EdgeIO_deployment/cluster$clusternum/cluster_orchestrator/;export SYSTEM_MANAGER_URL='$rootip'; export CLUSTER_NAME='cluster$clusternum'; export CLUSTER_LOCATION='cluster$clusternum'; docker-compose rm"
        clusternum=$((clusternum+1))
        echo "Next cluster $clusternum"
done

###### Instantiate cluster #####
echo "#####################"
echo "### Nodes shutdown ##"
echo "#####################"

###### Instantiate nodes ######
hostname=""
workernum=1
for cluster in $clusterlist; do
        echo "## Stopping workers for cluster $cluster ##"
        hostlist=""
        for host in $(cat $cluster.txt); do
                hostlist=$hostlist""$host" "
        done
        hostname=$cluster
        hostname_to_ip
        clusterip=$ip

        for host in $hostlist; do
                hostname=$host
                hostname_to_ip
                echo "Stopping: <$hostname,$ip> [If any error shows up... don't worry :D"
                ssh $1@$ip "num=\$(ps aux | grep start-up | grep -v grep | awk '{print \$2}'); sudo kill \$num"
		ssh $1@$ip "num=\$(ps aux | grep NetManager | grep -v grep | awk '{print \$2}'); sudo kill -9 \$num"
		ssh $1@$ip "num=\$(ps aux | grep node_engine | grep -v grep | awk '{print \$2}'); sudo kill -9 \$num"
                woerkernum=$((workernum+1))
		echo "Done!"
        done
done

