import os
from logs import logger
import requests


SYSTEM_MANAGER_ADDR = (
    "http://" + os.environ.get("SYSTEM_MANAGER_URL") + ":" + os.environ.get("SYSTEM_MANAGER_PORT")
)


def send_aggregated_info(my_id, data):
    logger.debug("Sending aggregated information to System Manager.")
    try:
        requests.post(SYSTEM_MANAGER_ADDR + "/api/information/" + str(my_id), json=data)
    except requests.exceptions.RequestException as e:
        logger.error(f"Calling System Manager /api/information not successful. Error: {e}")