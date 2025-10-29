FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

# The greenlet pip package requires gcc & g++ to work properly. Cryptography requires libffi.
RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ git libffi-dev

COPY requirements.txt /

RUN pip install -r requirements.txt --no-cache-dir && rm /requirements.txt

# Don't commit me!
COPY resource_abstractor_client /resource_abstractor_client
RUN pip install -e /resource_abstractor_client

COPY . /

# TRUE for verbose logging
ENV FLASK_DEBUG=TRUE

ENV ROOT_MONGO_URL=localhost
ENV ROOT_MONGO_PORT=10007

ENV ROOT_SCHEDULER_URL=localhost
ENV ROOT_SCHEDULER_PORT=7777

ENV NET_PLUGIN_URL=localhost
ENV NET_PLUGIN_PORT=10100

EXPOSE 10000
EXPOSE 50052

CMD ["python", "system_manager.py"]
