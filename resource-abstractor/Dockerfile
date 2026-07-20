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
ENV FLASK_DEBUG=TRUE
ENV LOG_LEVEL=INFO

ENV MONGO_URL=localhost
ENV MONGO_PORT=10007

EXPOSE 11011
EXPOSE 11012

CMD ["python", "resource_abstractor.py"]
