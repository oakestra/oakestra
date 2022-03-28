
#!/bin/bash

# docker-compose --file ../docker-compose-amd64.yml up -d

# create virtualenv
virtualenv --clear -p python3.8 .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

export FLASK_ENV=development
export FLASK_DEBUG=True # TRUE for logging

export MQTT_BROKER_URL=localhost
export MQTT_BROKER_PORT=10003

export CLUSTER_MONGO_URL=localhost
export CLUSTER_MONGO_PORT=10007

# export SYSTEM_MANAGER_URL=131.159.24.210
# export SYSTEM_MANAGER_URL=3.120.37.66
export SYSTEM_MANAGER_URL=localhost
export SYSTEM_MANAGER_PORT=10000

export CLUSTER_SCHEDULER_URL=localhost
export CLUSTER_SCHUEDLER_PORT=5555

export CLUSTER_NAME=cluster_thinkpad2
export CLUSTER_LOCATION=Garching2
export MY_PORT=8000

.venv/bin/python cluster_manager.py
