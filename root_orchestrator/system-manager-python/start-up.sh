#!/bin/bash

# create virtualenv
virtualenv --clear -p python3.8 .venv  # python3 -m venv .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

export FLASK_ENV=development
# export FLASK_DEBUG=False # TRUE for logging

export ROOT_MONGO_URL=172.17.0.0
export ROOT_MONGO_PORT=10007

export ROOT_SCHEDULER_URL=localhost
export ROOT_SCHEDULER_PORT=7777

export MY_PORT=10000

.venv/bin/python system_manager.py
