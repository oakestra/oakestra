FROM python:3.8.5-slim-buster

ADD requirements.txt /

RUN pip install -r requirements.txt

ADD . /


CMD ["python", "stupidserver.py"]
