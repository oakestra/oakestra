#!/usr/bin/python
import docker
from net_manager_requests import net_manager_docker_deploy,net_manager_docker_undeploy

docker_client = docker.from_env()


def start_container(job):
    image = job.get('image')
    name = job.get('job_name')
    port = job.get('port')
    commands = job.get('commands')

    try:
        # start container
        container = docker_client.containers.run(image, name=name, command=commands,
                                                 detach=True)
        container.pause()
        # assign address to the container
        address = net_manager_docker_deploy(job, str(container.id))
        if address == '':
            container.kill()
            raise Exception("Bad Network Address - NetManager error")
        container.unpause()

        return address
    except docker.errors.APIError as e:
        print("Oopps.. Docker API Error. {}")
        print(e)
        return None


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
