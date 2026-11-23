FROM python:3.10-slim-bookworm
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

COPY requirements.txt /

RUN pip install --no-cache-dir -r requirements.txt

COPY . /

ENV LOG_LEVEL=WARNING

ENV MONGO_URL=localhost
ENV MONGO_PORT=10007

EXPOSE 11011
EXPOSE 11012

CMD ["python", "resource_abstractor.py"]
