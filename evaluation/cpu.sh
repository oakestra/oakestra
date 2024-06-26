#!/bin/bash

echo "timestamp,%CPU,MEM" > cpumemoryusage.csv

i=0
while true
do
    i=$(($i+1))

    timestamp=$(($(date +%s%N)/1000000))
    cpu=$(awk '{u=$2+$4; t=$2+$4+$5; if (NR==1){u1=u; t1=t;} else print ($2+$4-u1) * 100 / (t-t1) "%"; }' <(grep 'cpu ' /proc/stat) <(sleep 1;grep 'cpu ' /proc/stat))

    totalmem=$(free -m | awk '/^Mem:/ { print $2 }')
    freemem=$(free -m | awk '/^Mem:/ { print $4 }')
    usedmem=$((totalmem - freemem))

    output="$timestamp,$cpu,$usedmem"
    echo $output >> cpumemoryusage.csv 2>&1
    echo -n "$i,"; echo "$output"
    
    sleep 1

done