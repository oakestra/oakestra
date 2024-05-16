FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

ADD requirements.txt /

RUN pip install -r requirements.txt

ADD . /

# TRUE for verbose logging
ENV FLASK_ENV=development
ENV FLASK_DEBUG=False

ENV ADDONS_MANAGER_ADDR=http://localhost:11101


CMD ["python", "addons_monitor.py"]
