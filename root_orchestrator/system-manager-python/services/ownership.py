"""
Which user an API call acts on behalf of.

Applications and services are scoped to their owner (the application's userId). Regular users only
reach their own; an Admin manages every user's applications, so for an Admin we act as the owner.
"""

import logging

from flask_jwt_extended import get_jwt
from resource_abstractor_client import app_operations, job_operations
from roles.securityUtils import Role

logger = logging.getLogger("system_manager")


def caller_is_admin():
    try:
        return Role.ADMIN in (get_jwt().get("roles") or [])
    except RuntimeError:  # outside a request with a verified JWT
        return False


def app_owner(appid, caller):
    """Owner of application <appid> when the caller is an Admin, otherwise the caller."""
    if not appid or not caller_is_admin():
        return caller
    app = app_operations.get_app(str(appid))
    if not app or not app.get("userId"):
        return caller
    return app["userId"]


def service_owner(serviceid, caller):
    """Owner of the application that service <serviceid> belongs to, for Admins."""
    if not serviceid or not caller_is_admin():
        return caller
    service = job_operations.get_job_by_id(str(serviceid))
    if not service or not service.get("applicationID"):
        return caller
    return app_owner(service["applicationID"], caller)
