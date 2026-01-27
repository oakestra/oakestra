FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

RUN apt-get update -y &&\
    apt-get install -y \
        curl \
        python3-dev \
        build-essential \
        git

COPY requirements.txt /

# Install appropriate requirements
ARG LIB_BRANCH=develop
ENV LIB_BRANCH=${LIB_BRANCH}

RUN pip install --no-cache-dir -r requirements.txt 

COPY . /

# ENV FLASK_APP=scheduler.py
ENV FLASK_ENV=development
# TRUE for verbose logging
ENV FLASK_DEBUG=FALSE

ENV SYSTEM_MANAGER_URL=3.120.37.66
ENV SYSTEM_MANAGER_PORT=10000
ENV SYSTEM_MANAGER_GRPC_PORT=50052

ENV CLUSTER_SERVICE_MANAGER_URL=localhost
ENV CLUSTER_SERVICE_MANAGER_PORT=10010

ENV CLUSTER_MONGO_URL=localhost
ENV CLUSTER_MONGO_PORT=10007

ENV CLUSTER_SCHEDULER_URL=localhost
ENV CLUSTER_SCHEDULER_PORT=5555

ENV MQTT_BROKER_URL=localhost
ENV MQTT_BROKER_PORT=10003
ENV MY_PORT=10100

CMD gunicorn --bind [::]:${MY_PORT} cluster_manager:app