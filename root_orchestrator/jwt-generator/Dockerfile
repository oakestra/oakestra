FROM python:3.13-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

# Cryptography requires libffi.
RUN apt-get update && apt-get install -y curl libffi-dev

COPY requirements.txt /

# hadolint ignore=DL3022
COPY --from=libraries . /libraries

RUN apt-get update -y && apt-get install apt-file -y && apt-file update && apt-get install -y python3-dev build-essential
RUN pip install --no-cache-dir -r requirements.txt \
    && pip install --no-cache-dir /libraries/oakestra_logging

COPY . /

ENV JWT_GENERATOR_URL=0.0.0.0
ENV JWT_GENERATOR_PORT=10011
ENV OAKESTRA_SERVICE_NAME=jwt_generator

EXPOSE 10011

CMD ["gunicorn", "--bind", "0.0.0.0:10011", "--logger-class", "oakestra_logging.gunicorn.StructuredGunicornLogger", "jwt_generator:app"]
