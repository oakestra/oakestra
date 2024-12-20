FROM python:3.8.5-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

WORKDIR /code

COPY requirements.txt /code

RUN pip install --no-cache-dir -r requirements.txt

COPY . /code

ENV ADDON_MARKETPLACE_PORT=11102
ENV ADDON_MARKETPLACE_MONGO_URL=localhost
ENV ADDON_MARKETPLACE_MONGO_PORT=10007

EXPOSE 11102

CMD ["python", "marketplace_manager.py"]
