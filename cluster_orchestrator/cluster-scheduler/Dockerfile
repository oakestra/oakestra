FROM python:3.8.5-slim-buster

RUN mkdir /app
ADD requirements.txt /app
ADD co_deployment_durations.txt /app
ADD co_replication_durations.txt /app
ADD sla_alarm_durations.txt /app
WORKDIR /app
RUN pip install -r requirements.txt

# Required for docker container to be able to use the ping command
RUN apt-get update && apt-get install software-properties-common -y && apt-get install iproute2 iputils-ping -y && apt-get install -y procps


ADD . /app

ENV FLASK_ENV=development
# TRUE for verbose logging
ENV FLASK_DEBUG=FALSE

ENV CLUSTER_MANAGER_URL=131.159.24.210
ENV CLUSTER_MANAGER_PORT=10000

ENV CLUSTER_MONGO_URL=131.159.24.210
ENV CLUSTER_MONGO_PORT=10007

ENV REDIS_ADDR=redis://:clusterRedis@localhost:6379

ENV MY_PORT=10005
EXPOSE 10005

CMD python cluster_scheduler.py & celery -A cluster_scheduler.celeryapp worker --concurrency=1 --loglevel=DEBUG --uid=nobody --gid=nogroup
