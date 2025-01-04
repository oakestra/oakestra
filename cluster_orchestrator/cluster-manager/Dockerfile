FROM python:3.10-slim-buster

LABEL org.opencontainers.image.source="https://github.com/oakestra/oakestra"
LABEL org.opencontainers.image.description="Oakestra Cluster Manager"
LABEL org.opencontainers.image.licenses="Apache-2.0"

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    python3-dev \
    build-essential \
    git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

ENV FLASK_ENV=development \
    FLASK_DEBUG=FALSE \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

ENV SYSTEM_MANAGER_URL=3.120.37.66 \
    SYSTEM_MANAGER_PORT=10000 \
    SYSTEM_MANAGER_GRPC_PORT=50052

ENV CLUSTER_SERVICE_MANAGER_URL=localhost \
    CLUSTER_SERVICE_MANAGER_PORT=10010

ENV CLUSTER_MONGO_URL=localhost \
    CLUSTER_MONGO_PORT=10007

ENV CLUSTER_SCHEDULER_URL=localhost \
    CLUSTER_SCHEDULER_PORT=5555

ENV MQTT_BROKER_URL=localhost \
    MQTT_BROKER_PORT=10003

EXPOSE $SYSTEM_MANAGER_PORT 10001

CMD ["python", "cluster_manager.py"]
