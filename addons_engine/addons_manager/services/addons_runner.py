import logging
import os
import threading

import docker
import requests
from db import addons_db

addons_manager = None

DEFAULT_PROJECT_NAME = os.environ.get("DEFAULT_PROJECT_NAME") or "root_orchestrator"
DEFAULT_NETWORK_NAME = f"{DEFAULT_PROJECT_NAME}_default"

ADDON_ID_LABEL = os.environ.get("ADDON_ID_LABEL") or "oak.addon.id"
ADDON_MANAGER_LABEL = os.environ.get("ADDON_MANAGER_KEY") or "oak.plugin.manager.id"
ADDON_SERVICE_NAME_LABEL = os.environ.get("ADDON_SERVICE_NAME_LABEL") or "oak.service.name"

MARKETPLACE_API = f"{os.environ.get('MARKETPLACE_ADDR')}/api/v1/marketplace"


class DockerAddonRunner:
    def __init__(self, manager_id):
        self._client = docker.from_env()
        self._manager_id = manager_id

    def _get_networks(self):
        return [network.name for network in self._client.networks.list()]

    def _create_network(self, network_name, **kwargs):
        self._client.networks.create(network_name, **kwargs)

    def _get_container(self, container_name):
        try:
            return self._client.containers.get(container_name)
        except Exception:
            return None

    def _stop_container(self, container):
        container.stop()

    def _remove_container(self, container):
        container.remove()

    def _get_container_networks(self, container):
        networks = container.attrs["NetworkSettings"]["Networks"]
        return list(networks.keys())

    def _get_container_ports(self, container):
        return container.attrs["NetworkSettings"]["Ports"]

    def maybe_create_networks(self, networks):
        available_networks = self._get_networks()
        unavailable_networks = list(set(networks) - set(available_networks))

        for network in unavailable_networks:
            self._create_network(network)

        # return newly created networks
        return unavailable_networks

    def _remove_image(self, image_name):
        try:
            self._client.images.remove(image_name)
        except docker.errors.DockerException as e:
            logging.warning(f"Failed to remove image {image_name}: {e}")
            return False

        return True

    def remove_addon_images(self, addon):
        services = addon.get("services", [])
        for service in services:
            image_uri = service.get("image_uri")
            self._remove_image(image_uri)

    def run_service(self, service, addon_id, project_name):
        # labels for basic structuring of the containers
        service["labels"] = service.get("labels", {})
        service["labels"]["com.docker.compose.project"] = project_name
        service["labels"]["com.docker.compose.service"] = service["service_name"]

        # Addon engine specific labels
        service["labels"][ADDON_ID_LABEL] = addon_id
        service["labels"][ADDON_MANAGER_LABEL] = self._manager_id
        service["labels"][ADDON_SERVICE_NAME_LABEL] = service["service_name"]

        service["networks"] = service.get("networks", [])
        if not service["networks"]:
            service["networks"].append(DEFAULT_NETWORK_NAME)

        self.maybe_create_networks(service["networks"])

        # Handling multiple networks is currently not needed.
        one_network = service["networks"][0]
        image_uri = service.get("image_uri")

        return self._client.containers.run(
            image_uri,
            name=service["service_name"],
            command=service.get("command", []),
            network=one_network,
            ports=service.get("ports", {}),
            environment=service.get("environment", {}),
            labels=service.get("labels", {}),
            detach=True,
        )

    def stop_addon(self, addon):
        services = addon.get("services", [])

        for service in services:
            container = self._get_container(service["service_name"])
            if container:
                self._stop_container(container)
                self._remove_container(container)
            else:
                logging.warning(f"Container-{service['service_name']} not found")

    def run_addon(self, addon, project_name=DEFAULT_PROJECT_NAME):
        """Runs the services for a given addon. addon object is modified in place.

        This function checks if the services for the addon are already running. If they are,
        it does nothing.
        If a similar service is running, it stops the existing container, and starts a new one
        with the service configuration.

        Args:
            addon (dict): The addon configuration. It should contain a 'services' key, which is a
            list of service configurations.
            Each service configuration is a dictionary that includes at least 'service_name' and
            'image_uri'.
            project_name (str, optional): The name of the project.
            Defaults to DEFAULT_PROJECT_NAME.

        Returns:
            dict: A dictionary with two keys:
                - 'failed_services': A list of services that failed to start. Each element is a
                service configuration dictionary.
                - 'new_containers': A list of the new containers that were started. Each element is
                a docker.models.containers.Container object.
        """

        addon_services = addon["services"]
        addon_id = str(addon.get("_id"))

        failed_services = []
        new_containers = []

        containers_to_stop = []
        services_to_run = []

        for service in addon_services:
            container_name = service.get("service_name")
            similar_container = self._get_container(container_name)

            # Container is already running. Do nothing.
            if (
                similar_container
                and similar_container.status == "running"
                and service["image_uri"] in similar_container.image.tags
            ):
                continue

            if similar_container:
                container_networks = self._get_container_networks(similar_container)
                container_ports = self._get_container_ports(similar_container)
                if container_networks:
                    service["networks"].extend(container_networks)

                # extending the ports of the image, but don't override the configured ones
                service["ports"] = service.get("ports", {})
                for key, value in container_ports.items():
                    if key not in service["ports"]:
                        service["ports"][key] = value

                containers_to_stop.append(similar_container)

            services_to_run.append(service)

        for container in containers_to_stop:
            self._stop_container(container)
            self._remove_container(container)

        # TODO handle case where container failed to start while it's alternative was stopped.
        for service in services_to_run:
            try:
                container = self.run_service(service, addon_id, project_name)
                new_containers.append(container)
            except docker.errors.APIError as e:
                logging.warning(f"Failed to run container: {e}")
                failed_services.append(service)

        return {
            "failed_services": failed_services,
            "new_containers": new_containers,
        }


def init_addon_manager(manager_id):
    global addons_manager

    addons_manager = DockerAddonRunner(manager_id)

    def run_active_addons():
        if addons_manager is None:
            logging.error("Container manager is not initialized")
            return

        addons = addons_db.find_active_addons()
        for addon in addons:
            addons_manager.run_addon(addon)

    threading.Thread(target=run_active_addons, daemon=True).start()

    return addons_manager


def stop_addons(addons):
    global addons_manager

    if not addons_manager:
        logging.error("Container manager not initialized")
        return

    for addon in addons:
        addons_manager.stop_addon(addon)


def stop_addon(addon):
    """Stop addons only stops the containers for the addon, but doesn't change its status."""
    global addons_manager

    if not addons_manager:
        logging.error("Container manager not initialized")
        return

    addons_manager.stop_addon(addon)


def run_addon(created_addon):
    result = addons_manager.run_addon(created_addon)
    all_services = created_addon.get("services", [])
    failed_services = result.get("failed_services", [])

    new_status = "enabled"
    if failed_services:
        logging.error(f"Failed to run services: {failed_services}")
        if len(failed_services) == len(all_services):
            new_status = "failed"
        else:
            new_status = "partially_enabled"

    # TODO: move db stuff outside by passing a success handler as an argument.
    # TODO: improve status message.
    addons_db.update_addon(str(created_addon["_id"]), {"status": new_status})


def install_addon(addon):
    global addons_manager

    if not addons_manager:
        logging.error("Container manager not initialized")
        return None

    marketplace_id = addon.get("marketplace_id")

    def get_addon_in_marketplace(marketplace_id):
        response = requests.get(f"{MARKETPLACE_API}/{marketplace_id}")
        response.raise_for_status()

        return response.json()

    marketplace_addon = get_addon_in_marketplace(marketplace_id)
    services = marketplace_addon.get("services", [])

    if not services:
        logging.error(f"Addon-{marketplace_id} has no services")
        return None

    addon["services"] = services
    addon["status"] = "installing"
    created_addon = addons_db.create_addon(addon)

    threading.Thread(target=run_addon, args=(created_addon,)).start()

    return created_addon
