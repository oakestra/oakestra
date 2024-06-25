import logging
import os
import time
from collections import defaultdict
from enum import Enum

import requests
from addons_runner.runner_types import RunnerTypes, get_runner

ADDONS_MANAGER_ADDR = (
    os.environ.get("ADDONS_MANAGER_ADDR") or "http://localhost:11101/api/v1/addons"
)

MAX_CONTAINER_RETRIES = os.environ.get("MAX_CONTAINER_RETRIES") or 1
CONTAINER_POLL_INTERVAL = os.environ.get("CONTAINER_POLL_INTERVAL") or 30

DEFAULT_PROJECT_NAME = os.environ.get("DEFAULT_PROJECT_NAME") or "root_orchestrator"
DEFAULT_NETWORK = f"{DEFAULT_PROJECT_NAME}_default"

ADDONS_ID_LABEL = os.environ.get("ADDONS_ID_LABEL") or "oak.addon.id"
ADDONS_SERVICE_NAME_LABEL = os.environ.get("ADDONS_SERVICE_NAME_LABEL") or "oak.service.name"


# TODO DUPLICATED CODE
class AddonStatusEnum(Enum):
    INSTALLING = "installing"
    DISABLE = "disabling..."
    DISABLED = "disabled"
    FAILED = "failed"
    ACTIVE = "active"
    PARTIALLY_ACTIVE = "partially_active"

    def __str__(self):
        return self.value


class AddonsMonitor:
    def __init__(self):
        self._running = True

        # TODO ----> add fail policy for a container failures in an addon, e.g. max retries
        # OR if to stop the addon completely if a single container fails.
        # TODO structure it by runner_type
        self._retry_containers = defaultdict(lambda: {})
        self._failed_containers = {}

    def _get_addon_runner(self, addon):
        runner_type = addon.get("runner", RunnerTypes.DOCKER)

        return get_runner(runner_type)

    def get_exit_code(self, container):
        return container.attrs["State"]["ExitCode"]

    def get_addons_from_manager(self, filters={}):
        response = requests.get(f"{ADDONS_MANAGER_ADDR}", params=filters)
        response.raise_for_status()

        return response.json()

    def maybe_create_networks(networks, runner_engine):
        available_networks = runner_engine.get_networks()
        unavailable_networks = list(set(networks) - set(available_networks))

        for network in unavailable_networks:
            runner_engine.create_network(network)

        # return newly created networks
        return unavailable_networks

    def run_addon(self, addon, done=None, **kwargs):
        runner_engine = self._get_addon_runner(addon)

        all_services = addon.get("services", [])

        running_services, failed_services = self.run_addon_services(
            str(addon.get("_id")),
            all_services,
            runner_engine,
        )

        new_status = AddonStatusEnum.ACTIVE
        status_details = {}
        if failed_services:
            logging.error(f"Failed to run services: {failed_services}")
            if len(failed_services) == len(all_services):
                new_status = AddonStatusEnum.FAILED
            else:
                new_status = AddonStatusEnum.PARTIALLY_ACTIVE
                status_details = {
                    "failed_services": [service.get("service_name") for service in failed_services]
                }

        if done:
            done(new_status, status_details, **kwargs)

        return running_services, failed_services

    def run_addon_services(self, addon_id, services, runner_engine):
        """Runs the services of an addon.

        This function checks if the services for the addon are already running. If they are,
        it does nothing.
        If a similar service is running, it stops the existing container, and starts a new one
        with the service configuration.

        Args:
            services (dict):
            Each service configuration is a dictionary that
                includes at least 'service_name' and 'image'.
            project_name (str, optional): The name of the project.
            Defaults to DEFAULT_PROJECT_NAME.

        Returns:
            tuple: A tuple containing two lists:
                - A list of the new containers that were started. Each element is
                a docker.models.containers.Container object.
                - A list of services that failed to start. Each element is a
                service configuration dictionary.
        """
        failed_services = []
        running_services = []

        services_to_stop = []
        services_to_run = []

        for service in services:
            container_name = service.get("service_name")
            similar_container = runner_engine.get_container(container_name)

            if (
                similar_container
                and runner_engine.is_container_running(similar_container)
                and runner_engine.is_container_running_image(
                    similar_container, service.get("image")
                )
            ):
                continue

            if similar_container:
                container_networks = runner_engine.get_container_networks(similar_container)
                container_ports = runner_engine.get_container_ports(similar_container)
                if container_networks:
                    service.get("networks").extend(container_networks)

                # extending the ports of the image, but don't override the configured ones
                service["ports"] = service.get("ports", {})
                for key, value in container_ports.items():
                    if key not in service["ports"]:
                        service["ports"][key] = value

                services_to_stop.append(similar_container)

            services_to_run(service)

        for container in services_to_stop:
            runner_engine.stop_container(container)

        for service in services_to_run:
            service["labels"][ADDONS_ID_LABEL] = addon_id
            service["labels"][ADDONS_SERVICE_NAME_LABEL] = service["service_name"]

            service["networks"] = service.get("networks", [])
            if not service["networks"]:
                service["networks"].append(DEFAULT_NETWORK)

            # TODO: don't create networks. if a network is not found, raise an error.
            self.maybe_create_networks(service["networks"], runner_engine)
            try:
                container = runner_engine.run_service(service, addon_id, DEFAULT_PROJECT_NAME)
                running_services.append(container)
            except Exception as e:
                logging.warning(f"Failed to run container: {e}")
                failed_services.append(service)

        return running_services, failed_services

    def stop_addon_services(self, services, runner_engine):
        for service in services:
            container_name = service.get("service_name")
            container = runner_engine.get_container(container_name)
            if container:
                runner_engine.stop_container(container)

    def stop_addon(self, addon, done=None):
        runner_engine = self._get_addon_runner(addon)

        services = addon.get("services", [])
        self.stop_addon_services(services, runner_engine)

        if done:
            done()

    def update_addon(self, addon_id, data):
        response = requests.patch(f"{ADDONS_MANAGER_ADDR}/{addon_id}", json=data)
        response.raise_for_status()

        return response.json()

    def get_addon_containers(self, addon_id, runner_engine):
        return runner_engine.get_containers(filters={"label": f"{ADDONS_ID_LABEL}={addon_id}"})

    def get_oak_addon_containers(self, runner_engine):
        # filter by key. Get containers created by the addon engine.
        return runner_engine.get_containers(filters={"label": ADDONS_ID_LABEL})

    def _handle_failed_container(self, container, addon_id, exit_code):
        curr_retries = self._retry_containers[addon_id].get(container.id, 0)

        if curr_retries >= MAX_CONTAINER_RETRIES:
            logging.error(
                (f"Addon-{addon_id}: container-{container.name} " f"exceeded max retries")
            )

            # remove container from retry list and add it to failed addons
            if not self._failed_containers.get(addon_id, None):
                self._failed_containers[addon_id] = set()

            self._failed_containers[addon_id].add(container.id)
            self._retry_containers[addon_id].pop(container.id, None)
        elif not self._failed_containers.get(addon_id, None):
            logging.info(
                f"Addon-{addon_id}: container-{container.name} exited with code {exit_code}"
            )
            self._retry_containers[addon_id][container.id] = curr_retries + 1

    def stop_monitoring(self):
        logging.info("Stopping monitoring of addons...")
        self._running = False

    def start_monitoring(self):
        logging.info("Starting monitoring of addons...")
        self._running = True

        # TODO toggling of running state is currently not utilized.
        while self._running:
            installing_addons = self.get_addons_from_manager(
                filters={"status": AddonStatusEnum.INSTALLING}
            )

            def on_installing_complete(status, details={}):
                try:
                    self.update_addon(
                        addon.get("_id"),
                        {"status": status, "status_details": details},
                    )
                except Exception as e:
                    # For consistency, stop the addon if we fail to update the status.
                    self.stop_addon(addon)
                    logging.error(f"Failed to update addon status: {e}")

            for addon in installing_addons:
                self.run_addon(addon, on_installing_complete)

            disable_addons = self.get_addons_from_manager(
                filters={"status": AddonStatusEnum.DISABLE}
            )

            def on_disable_complete():
                try:
                    self.update_addon(addon.get("_id"), {"status": AddonStatusEnum.DISABLED})
                except Exception as e:
                    logging.error(f"Failed to update addon status {AddonStatusEnum.DISABLED}: {e}")

            for addon in disable_addons:
                self.stop_addon(addon)

            # Find all supposedly running addons
            running_addons = self.get_addons_from_manager(
                filters={"status": AddonStatusEnum.ACTIVE}
            )

            for addon in running_addons:
                addon_id = addon.get("_id")
                runner_engine = self._get_addon_runner(addon)
                addon_containers = self.get_addon_containers(addon_id, runner_engine)

                for container in addon_containers:
                    exit_code = self.get_exit_code(container)
                    if exit_code != 0:
                        self._handle_failed_container(container, addon.get("_id"), exit_code)

                failed_containers = list(self._failed_containers.get(addon.get("_id"), set()))
                if failed_containers:
                    logging.info(f"Reporting failure of addon-{addon_id}-{container.id}.")
                    status = (
                        AddonStatusEnum.FAILED
                        if len(failed_containers) == len(addon_containers)
                        else AddonStatusEnum.PARTIALLY_ACTIVE
                    )
                    try:
                        self.update_addon(
                            addon_id,
                            {
                                "status": status,
                                "status_details": {"failed_services": failed_containers},
                            },
                        )

                        # we updated the status,
                        # so it's safe to remove it from the failed containers
                        for container_id in failed_containers:
                            # cleanup failed containers
                            runner_engine.stop_container_by_id(container_id)
                        self._failed_containers.pop(addon_id)
                    except Exception as e:
                        logging.error(f"Failed to update addon status {status}: {e}")

                # restart all containers that failed
                retry_containers = dict(self._retry_containers[addon_id])
                for container_id, _ in retry_containers:
                    container = runner_engine.get_container(container_id)
                    # Sanity check
                    if not container:
                        logging.warning(
                            f"Container '{container_id}' not found. Removing from retry list."
                        )
                        self._retry_containers[addon_id].pop(container_id, None)
                        # add to failed containers
                        self._failed_containers[addon_id].add(container_id)
                        continue

                    retry_count = self._retry_containers.get(container.id, 0)
                    logging.info(
                        f"Restarting container '{container.name}' "
                        f"for the ({retry_count}) time..."
                    )
                    container.restart()

            # poll every x seconds
            time.sleep(CONTAINER_POLL_INTERVAL)


addons_monitor = AddonsMonitor()
