FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

RUN mkdir /app
ADD requirements.txt /app
WORKDIR /app
RUN pip install -r requirements.txt

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
