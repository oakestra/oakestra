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
ENV FLASK_DEBUG=TRUE
ENV LOG_LEVEL=INFO

ENV MONGO_URL=localhost
ENV MONGO_PORT=10007

EXPOSE 11011
EXPOSE 11012

CMD ["python", "resource_abstractor.py"]
