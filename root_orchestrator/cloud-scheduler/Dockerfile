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
ENV C_FORCE_ROOT=TRUE

ENV SYSTEM_MANAGER_URL=131.159.24.210
ENV SYSTEM_MANAGER_PORT=10000

ENV CLOUD_MONGO_URL=131.159.24.210
ENV CLOUD_MONGO_PORT=10007

ENV RESOURCE_ABSTRACTOR_URL=131.159.24.210
ENV RESOURCE_ABSTRACTOR_PORT=11011

ENV REDIS_ADDR=redis://:cloudRedis@3.120.37.66:10009

ENV MY_PORT=10004
EXPOSE 10004

CMD python cloud_scheduler.py & celery -A cloud_scheduler.celeryapp worker --concurrency=1 --loglevel=DEBUG --uid=nobody --gid=nogroup
