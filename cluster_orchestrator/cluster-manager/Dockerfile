FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

ADD requirements.txt /

RUN apt-get update && apt-get install -y curl
RUN apt-get update -y && apt-get install apt-file -y && apt-file update && apt-get install -y python3-dev build-essential
RUN pip install -r requirements.txt

ADD . /

# ENV FLASK_APP=cloud_scheduler.py
ENV FLASK_ENV=development
# TRUE for verbose logging
ENV FLASK_DEBUG=FALSE

ENV SYSTEM_MANAGER_URL=3.120.37.66
ENV SYSTEM_MANAGER_PORT=10000

ENV CLUSTER_SERVICE_MANAGER_URL=localhost
ENV CLUSTER_SERVICE_MANAGER_PORT=10010

ENV CLUSTER_MONGO_URL=localhost
ENV CLUSTER_MONGO_PORT=10007

ENV CLUSTER_SCHEDULER_URL=localhost
ENV CLUSTER_SCHEDULER_PORT=5555

ENV MQTT_BROKER_URL=localhost
ENV MQTT_BROKER_PORT=10003

EXPOSE 10000
EXPOSE 10001

CMD ["python", "cluster_manager.py"]
