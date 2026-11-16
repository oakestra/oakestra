# --- Builder Stage ---
FROM python:3.10-slim-bookworm AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ git libffi-dev

COPY requirements.txt .

RUN pip install --no-cache-dir -r requirements.txt

RUN apt-get purge -y --auto-remove gcc g++ git libffi-dev && \
    apt-get clean

COPY . .

# --- Final Stage ---
FROM python:3.10-slim-bookworm AS final
LABEL org.opencontainers.image.source=https://github.com/oakestra/oakestra-net

WORKDIR /app

# artifacts from the builder stage
COPY --from=builder /app .
# installed dependencies
COPY --from=builder /usr/local/lib/python3.10/site-packages /usr/local/lib/python3.10/site-packages

ENV FLASK_ENV=development
# TRUE for verbose logging
ENV FLASK_DEBUG=False

ENV MY_PORT=10110
ENV MQTT_BROKER_PORT=10003
ENV MQTT_BROKER_URL=localhost
ENV ROOT_SERVICE_MANAGER_URL=localhost
ENV ROOT_SERVICE_MANAGER_PORT=10100
ENV SYSTEM_MANAGER_URL=localhost
ENV SYSTEM_MANAGER_PORT=10000
ENV CLUSTER_MONGO_URL=localhost
ENV CLUSTER_MONGO_PORT=10207

EXPOSE 10110

CMD ["python", "service_manager.py"]
