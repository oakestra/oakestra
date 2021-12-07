FROM python:3.8.5-slim-buster

ADD requirements.txt /

RUN pip install -r requirements.txt

ADD . /

# ENV FLASK_APP=cloud_scheduler.py
ENV FLASK_ENV=development
# TRUE for verbose logging
ENV FLASK_DEBUG=FALSE

ENV ROOT_SERVICE_MANAGER_URL=3.120.37.66
ENV ROOT_SERVICE_MANAGER_PORT=10000

ENV CLUSTER_SERVICE_MANAGER_PORT=10015

ENV CLUSTER_MONGO_URL=localhost
ENV CLUSTER_MONGO_PORT=10007

ENV MQTT_BROKER_URL=localhost
ENV MQTT_BROKER_PORT=10003

EXPOSE 10015

CMD ["python", "service_manager.py"]
