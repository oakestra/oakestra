import logging
import os
import time

import docker

ADDON_ID_LABEL = os.environ.get("ADDON_ID_LABEL") or "oak.addon.id"
ADDON_MANAGER_LABEL = os.environ.get("ADDON_MANAGER_KEY") or "oak.plugin.manager.id"

MAX_CONTAINER_RETRIES = os.environ.get("MAX_CONTAINER_RETRIES") or 1
CONTAINER_POLL_INTERVAL = os.environ.get("CONTAINER_POLL_INTERVAL") or 30


class AddonsMonitor:
    def __init__(
        self, addon_manager_id, failure_handler=lambda *args, **kwargs: print("Addon failed.")
    ):
        self._addon_manager_id = addon_manager_id
        self._failure_handler = failure_handler
        self._client = docker.from_env()

        # TODO ----> add fail policy for a container failures in an addon.
        self._retry_containers = {}
        self._failed_containers = {}

    def _get_exit_code(self, container):
        return container.attrs["State"]["ExitCode"]

    def get_oak_addon_containers(self, all=True):
        # filter by key-value. Get containers created by a particular addon engine instance.
        label = f"{ADDON_MANAGER_LABEL}={self._addon_manager_id}"

        return self._client.containers.list(filters={"label": label}, all=all)

    def _handle_failed_container(self, container, addon_id, exit_code):
        curr_retries = self._retry_containers.get(container.id, 0)

        if curr_retries >= MAX_CONTAINER_RETRIES:
            logging.error(
                (f"Addon-{addon_id}: container-{container.name} " f"exceeded max retries")
            )

            # remove container from retry list and add it to failed addons
            if not self._failed_containers.get(addon_id, None):
                self._failed_containers[addon_id] = []

            self._failed_containers[addon_id].append(container.id)
            self._retry_containers.pop(container.id, None)

            # report failure to addon manager
            logging.info(f"Reporting failure of addon-{addon_id}-{container.id} to addon manager")
            self._failure_handler(data=(addon_id, self._failed_containers[addon_id]))

        elif not self._failed_containers.get(addon_id, None):
            logging.info(
                f"Addon-{addon_id}: container-{container.name} exited with code {exit_code}"
            )
            self._retry_containers[container.id] = curr_retries + 1

    def start_monitoring(self):
        logging.info("Starting monitoring of addons")

        while True:
            # get all containers created by the addon engine, even exited ones.
            oak_containers = self.get_oak_addon_containers(all=True)
            logging.info(f"Found {len(oak_containers)} oak containers")

            for container in oak_containers:
                exit_code = self._get_exit_code(container)
                if exit_code != 0:
                    addon_id = container.labels.get(ADDON_ID_LABEL, None)
                    if not addon_id:
                        logging.error(f"Container {container.name} does not have an addon id label")
                        continue
                    self._handle_failed_container(container, addon_id, exit_code)

            # restart all containers that failed
            for container_id, _ in self._retry_containers.items():
                container = next((c for c in oak_containers if c.id == container_id), None)
                if container:
                    retry_count = self._retry_containers.get(container.id, 0)
                    logging.info(
                        f"Restarting container '{container.name}' for the ({retry_count}) time..."
                    )
                    container.restart()

            # poll every x seconds
            time.sleep(CONTAINER_POLL_INTERVAL)


addons_monitor = None


def init_addons_monitor(addon_manager_id, socketio):
    global addons_monitor

    def failure_hander(*args, **kwargs):
        socketio.emit("report_failure", *args, **kwargs)

    addons_monitor = AddonsMonitor(addon_manager_id, failure_handler=failure_hander)

    return addons_monitor
