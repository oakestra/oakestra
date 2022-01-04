#!/bin/bash

echo "Start removing container"
for ((i=1; i<=$1; i++));
  do docker container rm -f worker$i
done
echo "Done removing container"

