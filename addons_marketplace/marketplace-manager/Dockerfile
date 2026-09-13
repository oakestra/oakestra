FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

WORKDIR /code

COPY requirements.txt /code

# hadolint ignore=DL3022
COPY --from=libraries . /libraries

RUN pip install --no-cache-dir -r requirements.txt \
    && pip install --no-cache-dir /libraries/oakestra_logging

COPY . /code

ENV ADDON_MARKETPLACE_PORT=11102
ENV ADDON_MARKETPLACE_MONGO_URL=localhost
ENV ADDON_MARKETPLACE_MONGO_PORT=10007

EXPOSE 11102

CMD ["python", "marketplace_manager.py"]
