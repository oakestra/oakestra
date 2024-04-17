import logging
import os

from requests import exceptions

RESOURCE_ABSTRACTOR_ADDR = (
    f"http://{os.environ.get('RESOURCE_ABSTRACTOR_URL')}:"
    f"{os.environ.get('RESOURCE_ABSTRACTOR_PORT')}"
)


def make_request(method, api, **kwargs):
    url = f"{RESOURCE_ABSTRACTOR_ADDR}{api}"
    try:
        response = method(url, **kwargs)
        response.raise_for_status()
        return response.json()
    except exceptions.RequestException:
        logging.warning(f"Calling {url} not successful.")

    return None
