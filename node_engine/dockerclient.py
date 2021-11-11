#!/usr/bin/python
import docker
from docker.errors import NotFound
from net_manager_requests import net_manager_docker_deploy,net_manager_docker_undeploy

docker_client = docker.from_env()
CODE = 'code'
NAME = 'job_name'
PORT = 'port'
STATE = 'State'
STATUS = 'Status'
RUNNING = 'running'

def start_container(job):
    # image = job.get('image')
    image = job.get(CODE)
    name = job.get(NAME)
    port = job.get(PORT)
    port_out, port_in = port.split(":")
    try:
        # start container
        container = docker_client.containers.run(image, name=name, ports={port_in: port_out}, detach=True)
        # assign address to the container
        address = net_manager_docker_deploy(job, str(container.id))
        if address == '':
            raise Exception("Bad Address")
        print(container.id)
        print(address)
        return address, container.id
    except docker.errors.APIError as e:
        print("Oopps.. Docker API Error. {}")
        print(e)
        return None, None


def stop_container(container):
    container = docker_client.containers.get(container)
    container.remove(v=True, force=True)
    net_manager_docker_undeploy(str(container.id))
    return 0


def stop_all_running_containers():
    for container in docker_client.containers.list():
        container.stop()
        net_manager_docker_undeploy(str(container.id))
    return "Ok"


def list_container():
    return docker_client.containers.list()


def is_running(container_name):
    try:
        container = docker_client.containers.get(container_name)
    except NotFound:
        return False
    container_state = container.attrs[STATE]
    return container_state[STATUS] == RUNNING


def start_redis():
    container = docker_client.containers.run('redis', 'redis-server --requirepass workerRedis', name='worker_redis', ports={'6379/tcp': 6380}, detach=True)
    return container.id