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

export MY_PORT=10011

.venv/bin/python resource_abstractor.py &
