import logging
import os
from typing import Optional

from requests import exceptions

import inspect

RESOURCE_ABSTRACTOR_ADDR = (
    f"http://{os.environ.get('RESOURCE_ABSTRACTOR_URL')}:"
    f"{os.environ.get('RESOURCE_ABSTRACTOR_PORT')}"
)


def make_request(method, api: str, **kwargs) -> Optional[dict]:
    url = f"{RESOURCE_ABSTRACTOR_ADDR}{api}"
    try:
        response = method(url, **kwargs)
        response.raise_for_status()
        return response.json()
    except exceptions.RequestException as e:
        logging.warning(f"Calling {url} not successful.")
        print("error calling resource abstractor: ", e)
        print("Called by:", inspect.stack()[1].function)
        print("Called by:", inspect.stack()[2].function)
        print("Called by:", inspect.stack()[3].function)

    return None
