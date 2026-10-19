FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

COPY requirements.txt /

# hadolint ignore=DL3022
COPY --from=libraries . /libraries

RUN pip install --no-cache-dir -r requirements.txt \
    && pip install --no-cache-dir /libraries/oakestra_logging

COPY . /

# TRUE for verbose logging
ENV FLASK_ENV=development
ENV FLASK_DEBUG=False

ENV ADDONS_MANAGER_PORT=11101
ENV ADDONS_ENGINE_MONGO_URL=localhost
ENV ADDONS_ENGINE_MONGO_PORT=10007

ENV MARKETPLACE_ADDR=http://localhost:11102

CMD ["python", "addons_manager.py"]
