#!/bin/bash

# Install top level package in editable state (see setup.py)
export CLUSTER_MANAGER_IP=188.174.87.76
export CLUSTER_MANAGER_PORT=10000
export MQTT_BROKER_PORT=10003
export WORKER_PUBLIC_IP=188.174.87.76
export LAT=52.778016989191904
export LONG=8.073199727627696
export MY_PORT=3001
export REDIS_ADDR=redis://:workerRedis@localhost:6380
export VIVALDI_DIM=2
export GPS=TRUE

# Start celery worker
.venv/bin/celery worker -A celery_app:celery --autoscale=10,0

