FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra-net

ADD requirements.txt /

RUN apt-get update \
    && apt-get install -y g++ \
    && apt-get install -y --no-install-recommends gcc musl-dev linux-libc-dev libc6-dev \
    && rm -rf /var/lib/apt/lists/* \
    && pip install -r requirements.txt \
    && apt-get purge -y --auto-remove gcc musl-dev linux-libc-dev libc6-dev

ADD . /

# TRUE for verbose logging
ENV FLASK_DEBUG=FALSE

ENV CLOUD_MONGO_URL=localhost
ENV CLOUD_MONGO_PORT=10107
ENV MY_PORT=10100
ENV SYSTEM_MANAGER_URL=system_manager
ENV SYSTEM_MANAGER_PORT=10000

EXPOSE 10100

CMD ["python", "service_manager.py"]
