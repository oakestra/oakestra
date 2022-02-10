#!/usr/bin/python
import json.decoder

import docker
from docker.errors import NotFound
from app.blueprints.node_engine import net_manager_requests

docker_client = docker.from_env()
CODE = "code"
NAME = "job_name"
PORT = "port"
STATE = "State"
STATUS = "Status"
RUNNING = "running"
COMMANDS = "commands"

def start_container(job):
    # image = job.get("image")
    image = job.get(CODE)
    name = job.get(NAME)
    port = job.get(PORT)
    commands = job.get(COMMANDS)

    if ":" in port:
        port_in, port_out = port.split(":")
    else:
        port_in, port_out = port, None
    try:
        # start container
        container = docker_client.containers.run(image, name=name, ports={port_in: port_out}, command=commands, detach=True)
        container.pause()

        # assign address to the container
        address = net_manager_requests.net_manager_docker_deploy(job, str(container.id))
        if address == "":
            container.kill()
            raise Exception("Bad Network Address - NetManager error")

        container.unpause()
        print(container.id)
        print(address)

        if port_out is None:
            container.reload()
            port_out = container.attrs["NetworkSettings"]["Ports"]["5000/tcp"][0]["HostPort"]

        return address, container.id, port_out

    except docker.errors.APIError as e:
        print("Oopps.. Docker API Error. {}")
        print(e)
        return None, None, None


def stop_container(container):
    try:
        container = docker_client.containers.get(container)
        container.remove(v=True, force=True)
        net_manager_requests.net_manager_docker_undeploy(str(container.id))
    except docker.errors.NotFound as e:
        print(f"Container {container} not found")
    return 0


def stop_all_running_containers():
    for container in docker_client.containers.list():
        container.stop()
        net_manager_requests.net_manager_docker_undeploy(str(container.id))
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
    container = docker_client.containers.run("redis", "redis-server --requirepass workerRedis", name="worker_redis", ports={"6379/tcp": 6380}, detach=True)
    return container.id

def get_memory_usage_in_mb(container):
    stats = docker_client.containers.get(container).stats(stream=False)
    memory = stats["memory_stats"]["usage"]
    memory_in_mb = float(f"{memory/1024/1024:.2f}")
    return memory_in_mb


def get_cpu_usage(container):
    stats = docker_client.containers.get(container).stats(stream=False)
    usage_delta = stats["cpu_stats"]["cpu_usage"]["total_usage"] - stats["precpu_stats"]["cpu_usage"]["total_usage"]
    system_delta = stats["cpu_stats"]["system_cpu_usage"] - stats["precpu_stats"]["system_cpu_usage"]
    len_cpu = len(stats["cpu_stats"]["cpu_usage"]["percpu_usage"])

    percentage = (usage_delta / system_delta) * len_cpu * 100
    # percent = round(percentage, 2)
    print(percentage)