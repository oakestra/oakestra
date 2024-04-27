#!/bin/bash

# create virtualenv
virtualenv --clear -p python3.8 .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

export FLASK_ENV=development
export FLASK_DEBUG=False # TRUE for logging

PORT=11101

export ADDONS_MANAGER_PORT=$PORT
export ADDONS_ENGINE_MONGO_URL=localhost
export ADDONS_ENGINE_MONGO_PORT=10007

export MARKETPLACE_ADDR=http://localhost:11102

export ADDONS_MANAGER_ADDR=http://localhost:$PORT

.venv/bin/python addons-manager/addons_manager.py &
.venv/bin/python addons-monitor/addons_monitor.py &