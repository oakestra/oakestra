#!/bin/bash

if [[ $3 = "build" ]]; then
  echo "Start building image"
  docker build -t worker .
  echo "Done building image"
else
  echo "Reuse existing image"
fi

echo "Start running container"
for ((i=$1; i<=$2; i++));
  do docker run -d --name worker$i --network=cluster_orchestrator_default --privileged --cap-add=NET_ADMIN worker
  LATENCY=$((10 + $RANDOM % 100))
  echo worker${i}: $LATENCY
  docker exec worker$i tc qdisc add dev eth0 root netem delay ${LATENCY}ms

done
echo "Done running container"

