import threading

import docker
from db import marketplace_db
from oakestra_logging import get_logger

logger = get_logger(__name__)


def verify_addon(addon_id, addon):
    logger.info("Verifying addon", event_name="addon.verification.started", addon_id=addon_id)
    client = docker.from_env()
    for service in addon["services"]:
        image = service.get("image")
        image_id = None
        try:
            # TODO(ME): validate image instead of pulling it.
            logger.info("Pulling addon image", event_name="addon.image.pull_started", image=image)
            pulled_image = client.images.pull(image)
            image_id = pulled_image.id
            logger.info(
                "Addon image pulled",
                event_name="addon.image.pull_completed",
                image=image,
                image_id=pulled_image.id,
            )

            logger.info(
                "Addon verified", event_name="addon.verification.completed", addon_id=addon_id
            )
            marketplace_db.update_addon(
                addon_id, {"status": marketplace_db.StatusEnum.APPROVED.value}
            )
        except docker.errors.DockerException as exc:
            logger.warning(
                "Failed to pull addon image",
                event_name="addon.image.pull_failed",
                image=image,
                error_type=type(exc).__name__,
            )
            marketplace_db.update_addon(
                addon_id, {"status": marketplace_db.StatusEnum.VERIFICATION_FAILED.value}
            )
            return

        try:
            # This is not a failure, maybe the image is used by another service
            client.images.remove(image_id)
        except docker.errors.DockerException as exc:
            logger.warning(
                "Failed to remove verification image",
                event_name="addon.image.remove_failed",
                image_id=image_id,
                error_type=type(exc).__name__,
            )


def register_addon(addon):
    addon = marketplace_db.create_addon(
        {**addon, "status": marketplace_db.StatusEnum.UNDER_REVIEW.value}
    )
    addon_id = addon.get("_id")

    logger.info("Addon registered", event_name="addon.registered", addon_id=addon_id)
    threading.Thread(target=verify_addon, args=(addon_id, addon)).start()

    return addon
