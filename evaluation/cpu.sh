#!/bin/bash

echo "" > cpumemoryusage.csv
max=200

i=0
while [ $i -ne $max ]
do
    i=$(($i+1))
    timestamp=$(date)
    cpu=$(top -l 1 | awk '/CPU usage:/ {print $3}') # get CPU usage
    mem=$(top -l 1 | awk '/PhysMem:/ {print $2}') # get used memory
    output="$i,$timestamp,%CPU,$cpu,%MEM,$mem,$1"
    echo $output >> cpumemoryusage.csv 2>&1
    echo $output
    sleep 2
done