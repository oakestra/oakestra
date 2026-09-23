FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

# git is needed for the git+https://... entries in requirements.txt
# (shared libraries under libraries/).
RUN apt-get update && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /code

ARG LIB_BRANCH=develop
ENV LIB_BRANCH=${LIB_BRANCH}

COPY requirements.txt /code

RUN pip install --no-cache-dir -r requirements.txt

COPY . /code

ENV ADDON_MARKETPLACE_PORT=11102
ENV ADDON_MARKETPLACE_MONGO_URL=localhost
ENV ADDON_MARKETPLACE_MONGO_PORT=10007

EXPOSE 11102

CMD ["python", "marketplace_manager.py"]
