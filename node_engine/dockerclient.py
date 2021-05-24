#!/usr/bin/python

import docker
import mqtt_client
import node_engine

docker_client = docker.from_env()


def start_container(image, name, port, job_id):
    try:
        # start container
        container = docker_client.containers.run(image, name=name, ports={port: None}, detach=True)
        # assign address to the container
        # TODO
        address = '172.19.0.1'  # placeholder
        mqtt_client.publish_deploy_status(node_engine.node_info.id,job_id,'DEPLOYED',address)
        print(container.id)
        return "ok"
    except docker.errors.APIError:
        mqtt_client.publish_deploy_status(node_engine.node_info.id, job_id, 'FAILED', '')
        print("Oopps.. Docker API Error. {}")
        return -1


def stop_container(container):
    container = docker_client.containers.get(container)
    container.remove(v=True, force=True)
    return 0


def stop_all_running_containers():
    for container in docker_client.containers.list():
        container.stop()
    return "Ok"


def list_container():
    return docker_client.containers.list()
