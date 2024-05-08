FROM python:3.10-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

# The greenlet pip package requires gcc & g++ to work properly.
RUN apt-get update && apt-get install -y --no-install-recommends gcc g++

ADD requirements.txt /

RUN pip install -r requirements.txt --no-cache-dir && rm /requirements.txt

ADD . /

# TRUE for verbose logging
ENV FLASK_DEBUG=TRUE

ENV CLOUD_MONGO_URL=localhost
ENV CLOUD_MONGO_PORT=10007

ENV CLOUD_SCHEDULER_URL=localhost
ENV CLOUD_SCHEDULER_PORT=7777

ENV NET_PLUGIN_URL=localhost
ENV NET_PLUGIN_PORT=10100

EXPOSE 10000
EXPOSE 50052

CMD ["python", "system_manager.py"]
