import logging
import threading

import docker
from db import marketplace_db


def verify_addon(addon_id, addon):
    logging.info(f"verifying addon-{addon_id}...")
    client = docker.from_env()
    for service in addon["services"]:
        image = service.get("image")
        image_id = None
        try:
            # TODO validate image instead of pulling it.
            logging.info(f"Pulling image: {image}")
            pulled_image = client.images.pull(image)
            image_id = pulled_image.id
            logging.info(f"Image pulled: {pulled_image}")

            logging.info(f"Addon-{addon_id} verified")
            marketplace_db.update_addon(
                addon_id, {"status": marketplace_db.StatusEnum.APPROVED.value}
            )
        except docker.errors.DockerException as e:
            logging.warning(f"Failed to pull {image}", exc_info=e)
            marketplace_db.update_addon(
                addon_id, {"status": marketplace_db.StatusEnum.VERIFICATION_FAILED.value}
            )
            return

        try:
            # This is not a failure, maybe the image is used by another service
            client.images.remove(image_id)
        except docker.errors.DockerException as e:
            logging.warning(f"Failed to remove image {image_id}", exc_info=e)


def register_addon(addon):
    addon = marketplace_db.create_addon(
        {**addon, "status": marketplace_db.StatusEnum.UNDER_REVIEW.value}
    )
    addon_id = addon.get("_id")

    logging.info(f"Addon-{addon_id} registered.")
    threading.Thread(target=verify_addon, args=(addon_id, addon)).start()

    return addon
