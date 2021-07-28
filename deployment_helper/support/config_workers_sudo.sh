if [ -z "$1" ]
  then
    	echo "Error, command syntax is: script.sh username"
	exit 1
fi

#Enable sudo on all the machines
echo "Please insert sudo password for the remote host"
first=1
read -s password
list=""
while IFS=, read hostname ip; do
	if [[ "$first" -eq 1 ]]; then
		first=0
		scp utility/sudonopass.sh $1@$ip:/home/$1/
	fi
	list=$list""$ip" "	
done < node.txt

for ip in $list; do
	echo "Enabling sudo with no pass on $ip"
    	ssh -oStrictHostKeyChecking=no $1@$ip "echo '$password'|./sudonopass.sh $1"
done
