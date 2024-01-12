#!/bin/bash

virtualenv --clear -p python3.8 .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

export FLASK_ENV=development
export FLASK_DEBUG=False

export CLOUD_MONGO_URL=localhost
export CLOUD_MONGO_PORT=10007

export RESOURCE_ABSTRACTOR_PORT=11011

.venv/bin/python resource_abstractor.py
