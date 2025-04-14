FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

COPY requirements.txt /

RUN pip install --no-cache-dir -r requirements.txt

COPY . /

# TRUE for verbose logging
ENV FLASK_ENV=development
ENV FLASK_DEBUG=False

ENV ADDONS_MANAGER_ADDR=http://localhost:11101


CMD ["python", "addons_monitor.py"]
