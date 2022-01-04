#!/bin/bash

export FLASK_ENV=development
export FLASK_DEBUG=FALSE # TRUE for verbose logging #when True, MQTT log twice because Flask opens second reloader thread

export CLUSTER_MANAGER_IP=$CLUSTER_IP
export CLUSTER_MANAGER_PORT=10000
export MQTT_BROKER_PORT=10003
export WORKER_PUBLIC_IP=$MYIP
export LAT=52.778016989191904
export LONG=8.073199727627696
export MY_PORT=3001
export REDIS_ADDR=redis://:workerRedis@localhost:6380
export VIVALDI_DIM=2
export GPS=TRUE

.venv/bin/celery worker -A celery_app:celery
