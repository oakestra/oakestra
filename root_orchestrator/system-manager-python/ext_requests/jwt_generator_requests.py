import logging
import os
import requests
import time
from datetime import timedelta
from typing import Any, Optional
from flask_jwt_extended.typing import ExpiresDelta, Fresh

JWT_GENERATOR_ADDR = 'http://' + os.environ.get('JWT_GENERATOR_HOST', 'localhost') + ':' + str(
    os.environ.get('JWT_GENERATOR_PORT', '10011'))

def create_access_token(
    identity: Any,
    fresh: Fresh = False,
    expires_delta: Optional[ExpiresDelta] = None,
    additional_claims=None,
    additional_headers=None,
):
    request_addr = JWT_GENERATOR_ADDR + '/create'

    obj = {}
    if identity is not None:
        obj['identity'] = identity
    if fresh is not None and not isinstance(fresh, bool):
        obj['fresh'] = timedelta_to_dict(fresh)
    if expires_delta is not None and not isinstance(expires_delta, bool):
        obj['expires_delta'] = timedelta_to_dict(expires_delta)
    if additional_claims is not None:
        obj['additional_claims'] = additional_claims
    if additional_headers is not None:
        obj['additional_headers'] = additional_headers

    r = requests.post(request_addr, json=obj)
    r.raise_for_status()
    return r.json()['access_token']

def create_refresh_token(
    identity: Any,
    expires_delta: Optional[ExpiresDelta] = None,
    additional_claims=None,
    additional_headers=None,
):
    request_addr = JWT_GENERATOR_ADDR + '/refresh'

    obj = {}
    if identity is not None:
        obj['identity'] = identity
    if expires_delta is not None and not isinstance(expires_delta, bool):
        obj['expires_delta'] = timedelta_to_dict(expires_delta)
    if additional_claims is not None:
        obj['additional_claims'] = additional_claims
    if additional_headers is not None:
        obj['additional_headers'] = additional_headers
        
    r = requests.post(request_addr, json=obj)
    r.raise_for_status()
    return r.json()['refresh_token']

def get_public_key():
    logger = logging.getLogger()
    logger.info('new job: asking cloud_scheduler...')
    request_addr = JWT_GENERATOR_ADDR + '/key'
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
            logger.error(f'Calling JWT generator /key not successful: {e}')
            time.sleep(5)


def timedelta_to_dict(delta: timedelta):
    return {
        'days': delta.days,
        'seconds': delta.seconds,
        'microseconds': delta.microseconds,
    }