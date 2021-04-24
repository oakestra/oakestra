#!/bin/bash

# docker run -p 6379:6379 -d redis

# create virtualenv
virtualenv --clear -p python3.8 .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

export FLASK_ENV=development
export FLASK_DEBUG=False # TRUE for logging

export CLUSTER_MONGO_URL=localhost
export CLUSTER_MONGO_PORT=10007

export CLUSTER_MANAGER_URL=localhost
export CLUSTER_MANAGER_PORT=9000

export REDIS_ADDR=redis://localhost:6379

export MY_PORT=5555

.venv/bin/celery -A cluster_scheduler.celeryapp worker --concurrency=1 --loglevel=DEBUG &

.venv/bin/celery -A cluster_scheduler.celeryapp beat --loglevel=DEBUG &

.venv/bin/python cluster_scheduler.py &
