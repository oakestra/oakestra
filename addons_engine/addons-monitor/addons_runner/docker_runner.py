import logging

import docker

# from addons_runner.generic_runner import IRunner


class DockerRunner:
    def __init__(self):
        self._client = docker.from_env()

    def get_networks(self):
        return [network.name for network in self._client.networks.list()]

    def create_network(self, network_name, **kwargs):
        self._client.networks.create(network_name, **kwargs)

    def get_exit_code(self, container):
        return container.attrs["State"]["ExitCode"]

    def get_label(self, container, key):
        return container.labels.get(key)

    def get_containers(self, filters={}):
        return self._client.containers.list(filters=filters, all=all)

    def get_container(self, container_name):
        try:
            return self._client.containers.get(container_name)
        except Exception:
            return None

    def stop_container(self, container):
        container.stop()
        container.remove()

    def stop_container_by_id(self, container_id):
        container = self.get_container(container_id)
        if container:
            self.stop_container(container)

    def get_container_networks(self, container):
        networks = container.attrs["NetworkSettings"]["Networks"]
        return list(networks.keys())

    def get_container_ports(self, container):
        return container.attrs["NetworkSettings"]["Ports"]

    def remove_image(self, image_name):
        try:
            self._client.images.remove(image_name)
        except docker.errors.DockerException as e:
            logging.warning(f"Failed to remove image {image_name}: {e}")
            return False

        return True

    def run_service(self, service, project_name):
        # labels for basic structuring of the containers
        service["labels"] = service.get("labels", {})
        service["labels"]["com.docker.compose.project"] = project_name
        service["labels"]["com.docker.compose.service"] = service["service_name"]

        # Handling multiple networks is currently not needed.
        one_network = service["networks"][0]
        image = service.get("image")

        return self._client.containers.run(
            image,
            name=service["service_name"],
            command=service.get("command", []),
            network=one_network,
            ports=service.get("ports", {}),
            environment=service.get("environment", {}),
            labels=service.get("labels", {}),
            detach=True,
        )

    def is_container_running(self, container):
        return container and container.status == "running"

    def is_container_running_image(self, container, image_name):
        return container and image_name in container.image.tags
