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
echo "### Root Startup  ###"
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
        echo "Executing docker-compose up on node: <$hostname,$ip>"
	ssh $1@$ip "cd EdgeIO_deployment/root/root_orchestrator/; docker-compose up --build -d"
	rootip=$ip
done

echo "\n"
echo "#####################"
echo "##Clusters Startup###"
echo "#####################"

clusterlist=""
for cluster in $(cat cluster.txt); do
        clusterlist=$clusterlist""$cluster" "
done


###### Instantiate cluster #####
hostname=""
clusternum=1
for cluster in $clusterlist; do
        hostname=$cluster
        hostname_to_ip
        echo "Executing docker-compose up on cluster$clusternum node: <$hostname,$ip>"
	ssh $1@$ip "cd EdgeIO_deployment/cluster$clusternum/cluster_orchestrator/ && export SYSTEM_MANAGER_URL='$rootip' && export CLUSTER_NAME='cluster$clusternum' && export CLUSTER_LOCATION='cluster$clusternum' && docker-compose up --build -d"
	clusternum=$((clusternum+1))
	echo "Next cluster $clusternum"
done

echo "\n"
echo "#####################"
echo "### Nodes Startup ###"
echo "#####################"

###### Instantiate nodes ######
hostname=""
workernum=1
for cluster in $clusterlist; do
	echo "## Starting workers for cluster $cluster ##"
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
		echo "Running: <$hostname,$ip>"
		ssh $1@$ip "cd EdgeIO_deployment/worker$workernum/node_engine/; export CLUSTER_MANAGER_IP='$clusterip'; sh -c 'nohup ./start-up.sh amd64 > logs.log 2>&1 &'"
		woerkernum=$((workernum+1))
	done
done
