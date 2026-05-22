FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

# The greenlet pip package requires gcc & g++ to work properly. Cryptography requires libffi.
RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ git libffi-dev

COPY requirements.txt /
COPY --from=libs oakestra_utils_library /libs/oakestra_utils_library
COPY --from=libs resource_abstractor_client /libs/resource_abstractor_client

RUN pip install --no-cache-dir -r requirements.txt /libs/oakestra_utils_library /libs/resource_abstractor_client \
    && rm /requirements.txt

COPY . /

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
