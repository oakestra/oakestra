ARG BASE_DIR=/oakestra
ARG APP_DIR=$BASE_DIR/cluster_orchestrator/cluster-manager
ARG LIB_DIR=$BASE_DIR/libraries

# The builder starts from the same image as the runtime stage, not ghcr.io/astral-sh/uv:*:
# the uv images only ship linux/amd64 and linux/arm64, while we also build linux/arm/v7.
# uv itself is installed from PyPI, which has armv7 wheels.
FROM python:3.10-slim-trixie AS builder

ARG BASE_DIR
ARG APP_DIR
ARG LIB_DIR
ARG UV_VERSION=0.13.9

# Compilers and headers for dependencies without prebuilt wheels on every platform
# (e.g. grpcio, greenlet and cryptography on linux/arm/v7). Only the builder needs them.
RUN apt-get update \
    && apt-get --yes install --no-install-recommends \
      build-essential \
      libffi-dev \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir "uv==${UV_VERSION}"

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_NO_DEV=1 \
    UV_PYTHON_DOWNLOADS=never

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
