"""
Federated login: users sign in with an external identity provider instead of an Oakestra password.

Off unless FEDERATED_LOGIN_AUTHORIZE_URL is set (see override-federated-login.yml). Flow:
  1. A client (e.g. a dashboard) obtains a token from the identity provider.
  2. It sends that token to POST /api/auth/federated.
  3. The system manager asks the provider whether the token's owner may use this cluster:
       POST <FEDERATED_LOGIN_AUTHORIZE_URL>
       Authorization: Bearer <provider token>
       {"cluster_id": <FEDERATED_LOGIN_CLUSTER_ID>,
        "cluster_secret": <FEDERATED_LOGIN_CLUSTER_SECRET>}
     and expects 200 {"username": str, "email": str (optional), "roles": [Oakestra role, ...]}.
  4. The user is created (without password) or updated in the root organization with those roles,
     and receives regular Oakestra access/refresh tokens.

INTERNAL_LOGIN=false additionally disables password login, registration and password changes.
"""

import logging
import os
import re
from datetime import datetime, timedelta
from functools import wraps

import requests
from ext_requests import organization_db, user_db
from roles import securityUtils
from roles.securityUtils import Role

logger = logging.getLogger("system_manager")

KNOWN_ROLES = {Role.ADMIN, Role.ORGANIZATION_ADMIN, Role.APP_Provider, Role.INF_Provider}
USERNAME_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._@+-]{0,127}$")
DEFAULT_REFRESH_MINUTES = 60


class FederatedLoginError(Exception):
    def __init__(self, status, message):
        super().__init__(message)
        self.status = status
        self.message = message


def _flag(name, default):
    value = os.environ.get(name)
    if value is None or value.strip() == "":
        return default
    return value.strip().lower() not in ("false", "0", "no", "off")


def federated_login_enabled():
    return os.environ.get("FEDERATED_LOGIN_AUTHORIZE_URL", "").strip() != ""


def internal_login_enabled():
    return _flag("INTERNAL_LOGIN", True)


def login_methods():
    return {"internal": internal_login_enabled(), "federated": federated_login_enabled()}


def refresh_expires():
    try:
        minutes = int(os.environ.get("FEDERATED_LOGIN_REFRESH_MINUTES", DEFAULT_REFRESH_MINUTES))
    except ValueError:
        minutes = DEFAULT_REFRESH_MINUTES
    return timedelta(minutes=max(minutes, 1))


def log_auth_configuration():
    methods = login_methods()
    logger.info("Login methods: %s", methods)
    if not methods["internal"] and not methods["federated"]:
        logger.critical(
            "INTERNAL_LOGIN=false but FEDERATED_LOGIN_AUTHORIZE_URL is not set: nobody can log in"
        )


def internal_login_only(fn):
    """Rejects the request with 403 when password-based login is disabled on this cluster."""

    @wraps(fn)
    def wrapper(*args, **kwargs):
        if not internal_login_enabled():
            return {
                "message": "Password login is disabled on this cluster; use federated login"
            }, 403
        return fn(*args, **kwargs)

    return wrapper


def authorize_with_provider(provider_token):
    """Asks the identity provider who the token belongs to and which roles it has here."""
    url = os.environ.get("FEDERATED_LOGIN_AUTHORIZE_URL", "").strip()
    body = {
        "cluster_id": os.environ.get("FEDERATED_LOGIN_CLUSTER_ID", ""),
        "cluster_secret": os.environ.get("FEDERATED_LOGIN_CLUSTER_SECRET", ""),
    }
    try:
        r = requests.post(
            url,
            json=body,
            headers={"Authorization": "Bearer " + provider_token},
            timeout=10,
        )
    except requests.RequestException as e:
        logger.error("Federated login: identity provider unreachable: %s", e)
        raise FederatedLoginError(502, "Identity provider unreachable")

    if r.status_code == 401:
        raise FederatedLoginError(401, "Invalid or expired token")
    if r.status_code in (403, 404):
        raise FederatedLoginError(403, "You do not have access to this cluster")
    if r.status_code != 200:
        logger.error("Federated login: identity provider answered %s", r.status_code)
        raise FederatedLoginError(502, "Identity provider error")

    try:
        identity = r.json()
    except ValueError:
        raise FederatedLoginError(502, "Identity provider returned an invalid response")
    if not isinstance(identity, dict):
        raise FederatedLoginError(502, "Identity provider returned an invalid response")

    username = identity.get("username")
    if not isinstance(username, str) or not USERNAME_RE.match(username):
        raise FederatedLoginError(502, "Identity provider returned an invalid username")
    roles = identity.get("roles")
    roles = [r for r in roles if r in KNOWN_ROLES] if isinstance(roles, list) else []
    if not roles:
        raise FederatedLoginError(403, "You do not have access to this cluster")
    email = identity.get("email") if isinstance(identity.get("email"), str) else ""
    return {"username": username, "email": email, "roles": sorted(set(roles))}


def provision_user(identity, organization_id):
    """Creates the federated user on first login and syncs its roles on every login."""
    user = user_db.mongo_get_user_by_name(identity["username"])
    if user is None:
        user_id = str(
            user_db.mongo_save_user_without_roles(
                {
                    "name": identity["username"],
                    "email": identity["email"],
                    "password": None,
                    "federated": True,
                    "created_at": datetime.now().strftime("%d/%m/%Y %H:%M"),
                }
            )
        )
    elif not user.get("federated"):
        # Never let an external identity take over a local account (e.g. "Admin").
        raise FederatedLoginError(409, "A local user with this name already exists")
    else:
        user_id = str(user["_id"])
        if identity["email"] and identity["email"] != user.get("email"):
            user_db.mongo_update_user(user_id, {"email": identity["email"]})

    organization_db.mongo_add_user_role_to_organization(user_id, organization_id, identity["roles"])


def federated_login(provider_token):
    if not federated_login_enabled():
        raise FederatedLoginError(404, "Federated login is not enabled on this cluster")
    if not isinstance(provider_token, str) or provider_token.strip() == "":
        raise FederatedLoginError(400, "Missing token")

    identity = authorize_with_provider(provider_token.strip())
    organization = organization_db.mongo_get_organization_by_name("root")
    organization_id = str(organization["_id"])
    provision_user(identity, organization_id)

    claims = {
        "user": identity["username"],
        "roles": identity["roles"],
        "organization": organization_id,
        "federated": True,
    }
    logger.info("Federated login: %s with roles %s", identity["username"], identity["roles"])
    return {
        "token": securityUtils.create_jwt_auth_access_token(
            identity=identity["username"], additional_claims=claims
        ),
        "refresh_token": securityUtils.create_jwt_auth_refresh_token(
            identity=identity["username"],
            additional_claims=claims,
            expires_delta=refresh_expires(),
        ),
    }
