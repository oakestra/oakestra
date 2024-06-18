#!/bin/bash

max=300

echo "time,%ContainerUsage" > containerusage.csv

container_ids=$(docker ps -q)

# Initialize total CPU usage to 0
total_cpu_usage=0

i=0
while [ $i -ne $max ]
do
    i=$(($i+1))

    if (( i % 5 == 0 )); then
        container_ids=$(docker ps -q)
    fi

    total_cpu_usage=0
    total_cpu_usage=$(echo "$container_ids" | xargs -I {} -P $(nproc) sh -c 'docker stats {} --no-stream --format "{{.CPUPerc}}" | tr -d "%" ' | paste -sd+ - | bc)

    timestamp=$(($(date +%s%N)/1000000))

    output="$timestamp,$total_cpu_usage%"
    echo $output >> containerusage.csv 2>&1
    echo -n "$i,"; echo "$output"

done