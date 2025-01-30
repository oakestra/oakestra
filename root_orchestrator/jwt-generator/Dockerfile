FROM python:3.10-slim-buster
LABEL org.opencontainers.image.source https://github.com/oakestra/oakestra

COPY requirements.txt /

RUN apt-get update && apt-get install -y curl
RUN apt-get update -y && apt-get install apt-file -y && apt-file update && apt-get install -y python3-dev build-essential
RUN pip install --no-cache-dir -r requirements.txt

COPY . /

ENV JWT_GENERATOR_URL=0.0.0.0
ENV JWT_GENERATOR_PORT=10011

EXPOSE 10011

CMD ["python", "jwt_generator.py"]