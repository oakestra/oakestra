#!/bin/bash

echo "" > cpumemoryusage.csv
max=200

i=0
while [ $i -ne $max ]
do
    i=$(($i+1))
    timestamp=$(date)
    cpu=$(grep 'cpu ' /proc/stat | awk '{usage=($2+$4)*100/($2+$4+$5)} END {print usage "%"}')
    totalmem=$(free -m | awk '/^Mem:/ { print $2 }')
    freemem=$(free -m | awk '/^Mem:/ { print $4 }')
    usedmem=$((totalmem - freemem))
    output="$i,$timestamp,%CPU,$cpu,%MEM,${usedmem}Mi,$1"
    echo $output >> cpumemoryusage.csv 2>&1
    echo $output
    sleep 2
done