#!/bin/bash

export FLASK_ENV=development
# export FLASK_DEBUG=False # TRUE for logging

export ROOT_MONGO_URL=localhost
export ROOT_MONGO_PORT=10007

export ROOT_SCHEDULER_URL=localhost
export ROOT_SCHEDULER_PORT=7777

export MY_PORT=10000

uv run src/system_manager/main.py
