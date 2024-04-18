FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

ADD requirements.txt /

RUN pip install -r requirements.txt

ADD . /

ENV FLASK_ENV=development

ENV CLOUD_MONGO_URL=localhost
ENV CLOUD_MONGO_PORT=10007

ENV ADDON_ENGINE_PORT=11011

EXPOSE 11101

CMD ["python", "addon_engine.py"]
