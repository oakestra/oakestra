echo "### Reading the current configuration file..."
clusters=0
nodes=0

FILE=cluster.txt
if ! [[ -f "$FILE" ]]; then
    	echo "$FILE does not exists."
	exit 1
fi


for cluster in $(cat cluster.txt); do
        echo "Cluster: $cluster #"
	clusters=$((clusters+1))
	for node in $(cat $cluster".txt"); do
        	echo "VM: $node #"
		nodes=$((nodes+1))
	done
done

echo "Cluster number: $clusters"
echo "Worker number: $nodes"



echo "### creating the directories inside the nodes ###"
echo "Please insert username: "
read username

echo "cloning the repo..."
git clone https://github.com/edgeIO/src -b release-0.1
echo "repo clone successfull or already present!"

mkdir EdgeIO_deployment
cd EdgeIO_deployment
mkdir root
for i in $(seq 1 $clusters)
do  
   mkdir "cluster"$i
done
for i in $(seq 1 $nodes)
do
   mkdir "worker"$i
done

#Copy the root orchestrator
cp -r ../src/root_orchestrator root/

#Copy the clusters 
for i in $(seq 1 $clusters)
do
   cp -r ../src/cluster_orchestrator  "cluster"$i"/"
done

#Copy the workers
for i in $(seq 1 $nodes)
do
   cp -r ../src/node_engine  "worker"$i"/"
done

cd ../

echo "Base dir generated!"

echo "Copying the new directory to the destination..."
while IFS=, read hostname ip; do 
	echo "ip: $ip"
	echo "hostname: $hostname"
	scp -r EdgeIO_deployment/ $username@$ip:/home/$username/
        ssh -oStrictHostKeyChecking=no -tt $username@$ip "chmod 777 EdgeIO_deployment; cd EdgeIO_deployment; chmod -R 777 cluster*"
	break 
done < node.txt

#Install python3.8 in each machine
echo "installing python 3.8 ..."

list=""
while IFS=, read hostname ip; do
        list=$list""$ip" "
done < node.txt

for ip in $list; do
        echo "ip: $ip"
        echo "hostname: $hostname"
        ssh -oStrictHostKeyChecking=no -tt $username@$ip "sudo apt install -y python3.8; sudo apt install -y python3-pip;sudo pip3 install virtualenv; alias pip=pip3; alias python=python3"
done

echo "Node setup finished successfully!!"


