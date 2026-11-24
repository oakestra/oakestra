"""
Login settings shared by password login, OIDC login and the management lock.

OIDC login is off unless OIDC_ISSUER is set (see override-federated-login.yml and users/oidc.py).
INTERNAL_LOGIN=false turns off password login, registration and password changes, and locks every
user/role/organization write: the identity provider owns users and roles then.
"""

import logging
import os
from datetime import timedelta
from functools import wraps

from flask_smorest import abort
from roles.securityUtils import Role

logger = logging.getLogger("system_manager")

KNOWN_ROLES = (Role.ADMIN, Role.ORGANIZATION_ADMIN, Role.APP_Provider, Role.INF_Provider)
MANAGEMENT_LOCKED_MESSAGE = "Users and roles are managed by your identity provider"

DEFAULT_SCOPES = "openid profile email offline_access"
DEFAULT_SESSION_MINUTES = 15
DEFAULT_GRACE_HOURS = 24


def _env(name, default=""):
    value = os.environ.get(name)
    if value is None or value.strip() == "":
        return default
    return value.strip()


def _flag(name, default):
    value = _env(name)
    if value == "":
        return default
    return value.lower() not in ("false", "0", "no", "off")


def _number(name, default, minimum):
    try:
        return max(float(_env(name, str(default))), minimum)
    except ValueError:
        logger.warning("%s is not a number, using %s", name, default)
        return default


def _list(name):
    return [v.strip() for v in _env(name).split(",") if v.strip()]


def internal_login_enabled():
    return _flag("INTERNAL_LOGIN", True)


def oidc_enabled():
    return _env("OIDC_ISSUER") != ""


class OidcSettings:
    """Snapshot of the OIDC_* environment, read on every use so tests (and restarts) see changes."""

    def __init__(self):
        self.issuer = _env("OIDC_ISSUER")
        self.client_id = _env("OIDC_CLIENT_ID")
        self.client_secret = _env("OIDC_CLIENT_SECRET")
        self.redirect_uris = _list("OIDC_REDIRECT_URIS")
        self.scopes = _env("OIDC_SCOPES", DEFAULT_SCOPES)
        self.username_claim = _env("OIDC_USERNAME_CLAIM", "preferred_username")
        self.roles_claim = _env("OIDC_ROLES_CLAIM", "roles")
        self.role_map = parse_role_map(_env("OIDC_ROLE_MAP"))
        self.provider_name = _env("OIDC_PROVIDER_NAME", "OIDC")
        self.allowed_return_hosts = [h.lower() for h in _list("OIDC_ALLOWED_RETURN_HOSTS")]
        self.signout_feed_url = _env("OIDC_SIGNOUT_FEED_URL")
        self.token_encryption_key = _env("OIDC_TOKEN_ENCRYPTION_KEY")
        self.session = timedelta(
            minutes=_number("FEDERATED_SESSION_MINUTES", DEFAULT_SESSION_MINUTES, 1)
        )
        self.grace = timedelta(
            hours=_number("FEDERATED_OUTAGE_GRACE_HOURS", DEFAULT_GRACE_HOURS, 0)
        )

    def missing(self):
        required = {
            "OIDC_CLIENT_ID": self.client_id,
            "OIDC_CLIENT_SECRET": self.client_secret,
            "OIDC_REDIRECT_URIS": self.redirect_uris,
        }
        return [name for name, value in required.items() if not value]


def settings():
    return OidcSettings()


def parse_role_map(value):
    """'provider:OakestraRole,...' -> {provider: {OakestraRole, ...}}; unknown targets ignored."""
    mapping = {}
    for pair in value.split(","):
        source, sep, target = pair.strip().rpartition(":")
        if not sep or not source.strip() or target.strip() not in KNOWN_ROLES:
            if pair.strip():
                logger.warning("OIDC_ROLE_MAP: ignoring %r", pair.strip())
            continue
        mapping.setdefault(source.strip(), set()).add(target.strip())
    return mapping


def login_methods():
    internal = internal_login_enabled()
    oidc = oidc_enabled()
    return {
        "internal": internal,
        "oidc": oidc,
        "provider_name": settings().provider_name if oidc else None,
        "user_management": internal,
    }


def log_auth_configuration():
    methods = login_methods()
    logger.info("Login methods: %s", methods)
    if methods["oidc"]:
        missing = settings().missing()
        if missing:
            logger.critical("OIDC_ISSUER is set but %s missing: OIDC login will fail", missing)
    if not methods["internal"] and not methods["oidc"]:
        logger.critical(
            "INTERNAL_LOGIN=false but OIDC_ISSUER is not set: nobody can log in "
            "(use `python -m users.emergency` for an emergency Admin session)"
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


def user_management_only(fn):
    """Rejects user/role/organization writes with 403 while the identity provider owns them."""

    @wraps(fn)
    def wrapper(*args, **kwargs):
        if not internal_login_enabled():
            abort(403, message=MANAGEMENT_LOCKED_MESSAGE)
        return fn(*args, **kwargs)

    return wrapper
