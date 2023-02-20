
#!/bin/bash

# docker-compose --file ../docker-compose.yml up -d

# create virtualenv
virtualenv --clear -p python3.8 .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

export FLASK_ENV=development
export FLASK_DEBUG=True # TRUE for logging

export CLUSTER_MONGO_URL=localhost
export CLUSTER_MONGO_PORT=10007

export ROOT_SERVICE_MANAGER_URL=localhost
export ROOT_SERVICE_MANAGER_PORT=10015

export CLUSTER_NAME=cluster_thinkpad2
export CLUSTER_LOCATION=Garching2
export MY_PORT=8000

.venv/bin/python cluster_manager.py
