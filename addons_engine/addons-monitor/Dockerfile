FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

# git is needed for the git+https://... entries in requirements.txt
# (shared libraries under libraries/).
RUN apt-get update && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

ARG LIB_BRANCH=develop
ENV LIB_BRANCH=${LIB_BRANCH}

COPY requirements.txt /

RUN pip install --no-cache-dir -r requirements.txt

COPY . /

# TRUE for verbose logging
ENV FLASK_ENV=development
ENV FLASK_DEBUG=False

ENV ADDONS_MANAGER_ADDR=http://localhost:11101


CMD ["python", "addons_monitor.py"]
