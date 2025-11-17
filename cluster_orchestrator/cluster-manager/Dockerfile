FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

RUN apt-get update -y &&\
    apt-get install -y \
        curl \
        python3-dev \
        build-essential \
        git

# Branch of the oakestra_utils_library to install
ARG LIB_BRANCH=develop
ENV LIB_BRANCH=$LIB_BRANCH

COPY requirements.txt /
RUN sed "s/@develop/@${LIB_BRANCH}/g" requirements.txt > reqs_subst.txt
RUN pip install --no-cache-dir -r reqs_subst.txt

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

EXPOSE $SYSTEM_MANAGER_PORT
EXPOSE 10001

CMD ["python", "cluster_manager.py"]
