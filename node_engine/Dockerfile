FROM ubuntu:18.04
LABEL org.opencontainers.image.source https://github.com/edgeIO/edgeio

ARG ARCHITECTURE="amd64"

ENV CLUSTER_MANAGER_IP localhost
ENV CLUSTER_MANAGER_PORT 10000
ENV MQTT_BROKER_URL localhost
ENV MQTT_BROKER_PORT 10003
ENV PUBLIC_WORKER_IP 192.168.0.0
ENV PUBLIC_WORKER_PORT 50103

RUN apt-get update && apt-get install -y software-properties-common gcc && add-apt-repository -y ppa:deadsnakes/ppa
RUN apt-get update && apt-get install -y python3.8 python3-distutils python3-pip python3-apt
RUN apt install -y net-tools
RUN apt install -y iproute2
RUN apt install -y unzip
RUN apt install -y git
RUN apt install docker.io -y
RUN apt-get update && apt-get clean && apt-get install -y curl openvpn bridge-utils
RUN mkdir /dev/net

ADD . /
RUN pip3 install -r requirements.txt

RUN unzip NetManager-v0.01.zip
RUN cp NetManager-v0.01/build/bin/amd64-NetManager /bin/NetManager

CMD ["touch /netmanager.log"]
#CMD ["NetManager-v0.01/build/bin/amd64-NetManager &>> /netmanager.log &"]
#CMD ["python3", "node_engine.py"]
CMD ["./start-up-container.sh"]






