import os
import time
from datetime import timedelta
from typing import Any, Optional

import requests
from flask_jwt_extended.typing import ExpiresDelta, Fresh
from oakestra_logging import get_logger

JWT_GENERATOR_ADDR = (
    "http://"
    + os.environ.get("JWT_GENERATOR_URL", "localhost")
    + ":"
    + str(os.environ.get("JWT_GENERATOR_PORT", "10011"))
)


def create_access_token(
    identity: Any,
    fresh: Fresh = False,
    expires_delta: Optional[ExpiresDelta] = None,
    additional_claims=None,
    additional_headers=None,
):
    request_addr = JWT_GENERATOR_ADDR + "/create"

    obj = {}
    if identity is not None:
        obj["identity"] = identity
    if fresh is not None and not isinstance(fresh, bool):
        obj["fresh"] = timedelta_to_dict(fresh)
    if expires_delta is not None and not isinstance(expires_delta, bool):
        obj["expires_delta"] = timedelta_to_dict(expires_delta)
    if additional_claims is not None:
        obj["additional_claims"] = additional_claims
    if additional_headers is not None:
        obj["additional_headers"] = additional_headers

    r = requests.post(request_addr, json=obj, timeout=10)
    r.raise_for_status()
    return r.json()["access_token"]


def create_refresh_token(
    identity: Any,
    expires_delta: Optional[ExpiresDelta] = None,
    additional_claims=None,
    additional_headers=None,
):
    request_addr = JWT_GENERATOR_ADDR + "/refresh"

    obj = {}
    if identity is not None:
        obj["identity"] = identity
    if expires_delta is not None and not isinstance(expires_delta, bool):
        obj["expires_delta"] = timedelta_to_dict(expires_delta)
    if additional_claims is not None:
        obj["additional_claims"] = additional_claims
    if additional_headers is not None:
        obj["additional_headers"] = additional_headers

    r = requests.post(request_addr, json=obj, timeout=10)
    r.raise_for_status()
    return r.json()["refresh_token"]


def get_public_key():
    logger = get_logger(__name__)
    logger.info("Requesting JWT public key", event_name="jwt.public_key.requested")
    request_addr = JWT_GENERATOR_ADDR + "/key"
    while True:
        try:
            r = requests.get(request_addr, timeout=5)
            r.raise_for_status()
            body = r.json()
            return body["public_key"]
        except requests.exceptions.HTTPError as exc:
            logger.warning(
                "JWT public-key request returned an error; retrying",
                event_name="jwt.public_key.retry",
                error_type=type(exc).__name__,
                retry_delay_seconds=5,
            )
            time.sleep(5)
        except requests.exceptions.RequestException as exc:
            logger.warning(
                "JWT public-key request failed; retrying",
                event_name="jwt.public_key.retry",
                error_type=type(exc).__name__,
                retry_delay_seconds=5,
            )
            time.sleep(5)


def timedelta_to_dict(delta: timedelta):
    return {
        "days": delta.days,
        "seconds": delta.seconds,
        "microseconds": delta.microseconds,
    }
