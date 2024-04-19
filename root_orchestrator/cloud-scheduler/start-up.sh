#!/bin/bash

# docker start redis || docker run -p 6379:6379 --name redis -d redis

# create virtualenv
virtualenv --clear -p python3.8 .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

export FLASK_ENV=development
export FLASK_DEBUG=False # TRUE for logging

export CLOUD_MONGO_URL=3.120.37.66
export CLOUD_MONGO_PORT=10007

export SYSTEM_MANAGER_URL=localhost
export SYSTEM_MANAGER_PORT=10000

export RESOURCE_ABSTRACTOR_URL=localhost
export RESOURCE_ABSTRACTOR_PORT=11011

export REDIS_ADDR=redis://:cloudRedis@3.120.37.66:10009

export MY_PORT=10004

.venv/bin/celery -A cloud_scheduler.celeryapp worker --concurrency=1 --loglevel=DEBUG &

.venv/bin/celery -A cloud_scheduler.celeryapp beat --loglevel=DEBUG &

.venv/bin/python cloud_scheduler.py &
