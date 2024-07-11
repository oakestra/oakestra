import logging
import threading

import docker
from db import marketplace_db


def verify_addon(addon_id, addon):
    logging.info(f"verifying addon-{addon_id}...")
    client = docker.from_env()
    for service in addon["services"]:
        image = service.get("image")
        try:
            logging.info(f"Getting image info: {image}")
            client.images.get_registry_data(image)

            logging.info(f"Addon-{addon_id} verified")
            marketplace_db.update_addon(
                addon_id, {"status": marketplace_db.StatusEnum.APPROVED.value}
            )
        except docker.errors.DockerException as e:
            logging.warning(f"Failed to get {image} data", exc_info=e)
            marketplace_db.update_addon(
                addon_id, {"status": marketplace_db.StatusEnum.VERIFICATION_FAILED.value}
            )


def register_addon(addon):
    addon = marketplace_db.create_addon(
        {**addon, "status": marketplace_db.StatusEnum.UNDER_REVIEW.value}
    )
    addon_id = addon.get("_id")

    logging.info(f"Addon-{addon_id} registered.")
    threading.Thread(target=verify_addon, args=(addon_id, addon)).start()

    return addon
