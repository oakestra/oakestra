FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

RUN apt-get update && apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*

COPY requirements.txt /

ARG LIB_BRANCH=develop

RUN pip install --no-cache-dir -r requirements.txt

COPY . /

# TRUE for verbose logging
ENV FLASK_ENV=development
ENV FLASK_DEBUG=False

ENV ADDONS_MANAGER_PORT=11101
ENV ADDONS_ENGINE_MONGO_URL=localhost
ENV ADDONS_ENGINE_MONGO_PORT=10007

ENV MARKETPLACE_ADDR=http://localhost:11102

CMD ["python", "addons_manager.py"]
