FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

ADD requirements.txt /

RUN pip install -r requirements.txt

ADD . /

# TRUE for verbose logging
ENV FLASK_ENV=development
ENV FLASK_DEBUG=TRUE

ENV CLOUD_MONGO_URL=localhost
ENV CLOUD_MONGO_PORT=10007

ENV RESOURCE_ABSTRACTOR_PORT=11011

EXPOSE 11011

CMD ["python", "resource_abstractor.py"]
