import logging
import threading

import docker
from db import marketplace_db


def verify_addon(addon_id, addon):
    logging.info(f"verifying addon-{addon_id}...")
    client = docker.from_env()
    for service in addon["services"]:
        image_uri = service.get("image_uri")
        try:
            # TODO utilize opencontainers image spec to verify image instead of docker
            logging.info(f"Pulling image: {image_uri}")
            service = client.images.pull(image_uri)
            logging.info(f"Image pulled: {image_uri}")

            client.images.remove(service.id)
        except docker.errors.ImageRemoveError as e:
            logging.warning(f"Failed to remove image {image_uri}: {e}")
        except docker.errors.DockerException as e:
            logging.warning(f"Failed to pull image {image_uri}: {e}")
            marketplace_db.update_addon(addon_id, {"status": "verification_failed"})
            return

    logging.info(f"Addon-{addon_id} verified")
    marketplace_db.update_addon(addon_id, {"status": "approved"})


def register_addon(addon):
    addon = marketplace_db.create_addon({**addon, "status": "under_review"})
    addon_id = addon.get("_id")

    logging.info(f"Addon-{addon_id} registered.")
    threading.Thread(target=verify_addon, args=(addon_id, addon)).start()

    return addon