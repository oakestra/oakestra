FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

ADD requirements.txt /

RUN pip install -r requirements.txt

ADD . /

ENV ADDON_MARKETPLACE_PORT=11102
ENV ADDON_MARKETPLACE_MONGO_URL=localhost
ENV ADDON_MARKETPLACE_MONGO_PORT=10007

EXPOSE 11102

CMD ["python", "addons_marketplace.py"]
