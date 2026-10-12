#!/bin/bash

python3 -m venv --clear .venv
source .venv/bin/activate

.venv/bin/pip install -r requirements.txt

export FLASK_ENV=development
export FLASK_DEBUG=False

export MONGO_URL=localhost
export MONGO_PORT=10007

.venv/bin/python resource_abstractor.py
