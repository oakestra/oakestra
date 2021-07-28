echo "Please Insert the following required information to generate brand new config files:"
echo "\n"



echo "#### Insert one after another all the nodes available #### \n"
echo "<hostname1>,<ip_node_1> \t"
read nodename
printf "" > node.txt
counter=2
while [ "$nodename" != '' ];
do
	echo "$nodename" >> node.txt
	echo "<hostname$counter>,<ip_node_$counter>"
	read nodename
	counter=$((counter+1))
done



echo "#### Insert the hostname of the root orchestrator's machine  #### \n"
echo "<root_machine_hostname>"
read nodename
printf "" > root.txt
echo "$nodename" >> root.txt


echo "#### Insert one after another all the cluster orchestrator's  machines  #### \n"
echo "<cluster_1_hostname>\t"
read nodename
printf "" > cluster.txt
counter=2
while [ "$nodename" != '' ];
do
        echo "$nodename" >> cluster.txt
        echo "<cluster_$counter _hostname>"
        read nodename
	counter=$((counter+1))
done

echo "#### Insert one after another all the nodes for each cluster ####"
for cluster in $(cat cluster.txt); do
	echo "Cluster: $cluster #"
	filename=$cluster.txt
	printf "" >"$filename"
	echo "<worker_1_hostname>"
	read nodename
	counter=2
	while [ "$nodename" != '' ];
	do
        	echo "$nodename" >>"$filename"
        	echo "<worker_$counter _hostname>"
        	read nodename
        	counter=$((counter+1))
	done
done
