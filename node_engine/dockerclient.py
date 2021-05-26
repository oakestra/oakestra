#!/usr/bin/python
import docker

docker_client = docker.from_env()


def start_container(image, name, port):
    try:
        # start container
        container = docker_client.containers.run(image, name=name, ports={port: None}, detach=True)
        # assign address to the container
        # TODO
        address = '172.19.0.1'  # placeholder
        print(container.id)
        return address
    except docker.errors.APIError as e:
        print("Oopps.. Docker API Error. {}")
        print(e)
        return None


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
