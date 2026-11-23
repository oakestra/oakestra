ARG BASE_DIR=/oakestra
ARG APP_DIR=$BASE_DIR/cluster_orchestrator/cluster-manager
ARG LIB_DIR=$BASE_DIR/libraries

FROM ghcr.io/astral-sh/uv:python3.10-trixie-slim AS builder

ARG BASE_DIR
ARG APP_DIR
ARG LIB_DIR

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_NO_DEV=1

COPY pyproject.toml uv.lock $BASE_DIR/

WORKDIR $BASE_DIR
RUN uv sync --frozen --no-install-workspace --package cluster-manager

COPY libraries/ $LIB_DIR/
COPY cluster_orchestrator/cluster-manager/ $APP_DIR/

WORKDIR $APP_DIR
RUN uv run --no-sync -m grpc_tools.protoc \
        -Isrc \
        --python_out=src \
        --grpc_python_out=src \
        --pyi_out=src \
        src/cluster_manager/proto/cluster_registration.proto \
    && uv sync --frozen --no-editable

FROM python:3.10-slim-trixie

ARG BASE_DIR

COPY --from=builder $BASE_DIR/.venv $BASE_DIR/.venv

ENV PATH="$BASE_DIR/.venv/bin:$PATH" \
    FLASK_DEBUG=0 \
    SYSTEM_MANAGER_URL=3.120.37.66 \
    SYSTEM_MANAGER_PORT=10000 \
    SYSTEM_MANAGER_GRPC_PORT=50052 \
    CLUSTER_SERVICE_MANAGER_ADDR=localhost \
    CLUSTER_SERVICE_MANAGER_PORT=10010 \
    CLUSTER_MONGO_URL=localhost \
    CLUSTER_MONGO_PORT=10007 \
    CLUSTER_SCHEDULER_URL=localhost \
    CLUSTER_SCHEDULER_PORT=5555 \
    MQTT_BROKER_URL=localhost \
    MQTT_BROKER_PORT=10003 \
    MY_PORT=10100 \
    LOG_LEVEL=INFO

LABEL org.opencontainers.image.source="https://github.com/oakestra/oakestra"

CMD gunicorn --bind [::]:${MY_PORT} cluster_manager.main:app
