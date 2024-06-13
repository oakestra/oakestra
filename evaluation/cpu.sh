#!/bin/bash

max=200

echo "time,%CPU,%MEM,%MaxCPUCore,%ContainerUsage" > cpumemoryusage.csv


# Initialize total CPU usage to 0
total_cpu_usage=0

i=0
while [ $i -ne $max ]
do
    i=$(($i+1))

    container_ids=$(docker ps -q)

    total_cpu_usage=0
    total_cpu_usage=$(echo "$container_ids" | xargs -I {} -P $(nproc) sh -c 'docker stats {} --no-stream --format "{{.CPUPerc}}" | tr -d "%" ' | paste -sd+ - | bc)

    timestamp=$(date "+%T")
    cpu=$(grep -P 'cpu ' /proc/stat | awk '{usage=($2+$4)*100/($2+$4+$5)} END {print usage "%"}')
    maxcpu=$(grep -P 'cpu\d+' /proc/stat | awk '{usage=($2+$4)*100/($2+$4+$5)} {if (max < usage) {max = usage; maxcore = $1}} END {print max "%"}')
    totalmem=$(free -m | awk '/^Mem:/ { print $2 }')
    freemem=$(free -m | awk '/^Mem:/ { print $4 }')
    usedmem=$((totalmem - freemem))
    output="$timestamp,$cpu,$usedmem,$maxcpu,$total_cpu_usage%"
    echo $output >> cpumemoryusage.csv 2>&1
    echo -n "$i,"; echo "$output"
done