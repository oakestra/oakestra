FROM python:3.8.5-slim-buster

ADD requirements.txt /

RUN pip install -r requirements.txt

ADD . /

# TRUE for verbose logging
ENV FLASK_DEBUG=FALSE

ENV CLOUD_MONGO_URL=131.159.24.210
ENV CLOUD_MONGO_PORT=10007

ENV CLOUD_SCHEDULER_URL=localhost
ENV CLOUD_SCHEDULER_PORT=7777

ENV NET_PLUGIN_URL=localhost
ENV NET_PLUGIN_PORT=10100

EXPOSE 10000

CMD ["python", "system_manager.py"]
