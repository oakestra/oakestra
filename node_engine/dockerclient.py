#!/usr/bin/python

import docker

docker_client = docker.from_env()


def start_container(image, name, port):
    try:
        container = docker_client.containers.run(image, name=name, ports={port: None}, detach=True)
        print(container.id)
        return "ok"
    except docker.errors.APIError:
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
