FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

WORKDIR /code

RUN apt-get update && apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*

COPY requirements.txt /code

ARG LIB_BRANCH=develop

RUN pip install --no-cache-dir -r requirements.txt

COPY . /code

ENV ADDON_MARKETPLACE_PORT=11102
ENV ADDON_MARKETPLACE_MONGO_URL=localhost
ENV ADDON_MARKETPLACE_MONGO_PORT=10007

EXPOSE 11102

CMD ["python", "marketplace_manager.py"]
