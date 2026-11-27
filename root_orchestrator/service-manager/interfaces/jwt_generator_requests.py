import logging
import os
import time

import requests

logger = logging.getLogger("root_service_manager")

JWT_GENERATOR_ADDR = (
    "http://"
    + os.environ.get("JWT_GENERATOR_URL", "localhost")
    + ":"
    + str(os.environ.get("JWT_GENERATOR_PORT", "10011"))
)


def get_public_key():
    logger.info("new job: asking root_scheduler...")
    request_addr = JWT_GENERATOR_ADDR + "/key"
    while True:
        try:
            r = requests.get(request_addr)
            r.raise_for_status()
            body = r.json()
            return body["public_key"]
        except requests.exceptions.HTTPError as e:
            logger.error(f"Error: {e}, retrying in 5 seconds...")
            time.sleep(5)
        except requests.exceptions.RequestException as e:
            logger.error(f"Calling JWT generator /key not successful: {e}")
            time.sleep(5)
