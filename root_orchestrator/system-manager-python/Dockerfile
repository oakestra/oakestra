FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

# The greenlet pip package requires gcc & g++ to work properly. Cryptography requires libffi.
RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ git libffi-dev

COPY requirements.txt /

# Oakestra shared libraries are pulled from the repo-local libraries/ folder via the
# "libraries" named build context (see docker-compose.yml / CI), not from a remote git repo.
# DL3022 is ignored below: "libraries" is a BuildKit named build context, not a FROM stage.
# hadolint ignore=DL3022
COPY --from=libraries . /libraries

# Install appropriate requirements
RUN pip install -r requirements.txt --no-cache-dir \
    && pip install --no-cache-dir /libraries/resource_abstractor_client /libraries/oakestra_utils_library \
    && rm /requirements.txt

COPY . /

RUN python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. proto/clusterRegistration.proto

# TRUE for verbose logging
ENV FLASK_DEBUG=FALSE

ENV ROOT_MONGO_URL=localhost
ENV ROOT_MONGO_PORT=10007

ENV ROOT_SCHEDULER_URL=localhost
ENV ROOT_SCHEDULER_PORT=7777

ENV NET_PLUGIN_URL=localhost
ENV NET_PLUGIN_PORT=10100

ENV LOG_LEVEL=INFO

ENV MY_PORT=10000

EXPOSE 10000
EXPOSE 50052

CMD gunicorn --bind [::]:${MY_PORT} system_manager:app
