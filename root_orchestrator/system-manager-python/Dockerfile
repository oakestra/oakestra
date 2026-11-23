ARG BASE_DIR=/oakestra
ARG APP_DIR=$BASE_DIR/root_orchestrator/system-manager-python
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
RUN uv sync --frozen --no-install-workspace --package system-manager

COPY libraries/ $LIB_DIR/
COPY root_orchestrator/system-manager-python/ $APP_DIR/

WORKDIR $APP_DIR
RUN uv run --no-sync -m grpc_tools.protoc \
        -Isrc \
        --python_out=src \
        --grpc_python_out=src \
        --pyi_out=src \
        src/system_manager/proto/cluster_registration.proto \
    && uv sync --frozen --no-editable

FROM python:3.10-slim-trixie

ARG BASE_DIR

COPY --from=builder $BASE_DIR/.venv $BASE_DIR/.venv

# TODO: Let the service create its upload directory under XDG_DATA_HOME itself.
RUN mkdir -p /files

RUN apt-get update \
    && apt-get --yes install --no-install-recommends \
      libffi-dev \
    && rm -rf /var/lib/apt/lists/*

ENV PATH="$BASE_DIR/.venv/bin:$PATH" \
    FLASK_DEBUG=0 \
    ROOT_MONGO_URL=localhost \
    ROOT_MONGO_PORT=10007 \
    ROOT_SCHEDULER_URL=localhost \
    ROOT_SCHEDULER_PORT=7777 \
    NET_PLUGIN_URL=localhost \
    NET_PLUGIN_PORT=10100 \
    MY_PORT=10000 \
    LOG_LEVEL=INFO

LABEL org.opencontainers.image.source="https://github.com/oakestra/oakestra"

CMD gunicorn --bind [::]:${MY_PORT} system_manager.main:app
