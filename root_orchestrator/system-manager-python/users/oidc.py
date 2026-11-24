"""
OIDC login: the system manager is a confidential OpenID Connect client (Authorization Code + PKCE
S256, state and nonce) of the provider at OIDC_ISSUER, built on Authlib. It validates the provider's
ID token and then issues normal Oakestra tokens (claims user, roles, organization, federated, sid).

Users are keyed on the ID token's iss+sub (stored as oidc_iss/oidc_sub on the user document). The
username is chosen once at the first sign-in and never changes, because applications are owned by
username. Roles come from the OIDC_ROLES_CLAIM claim (optionally renamed with OIDC_ROLE_MAP); only
Oakestra role names survive and a user without any is refused. Every sign-in and every renewal
replaces the user's roles in the root organization with what the provider says.
"""

import logging
import secrets
import threading
import time
from datetime import timedelta
from urllib.parse import urlsplit, urlunsplit

import ext_requests.mongodb_client as mongodb
import requests
from authlib.common.errors import AuthlibBaseError
from authlib.integrations.requests_client import OAuth2Session, OAuthError
from authlib.jose import JsonWebKey, JsonWebToken
from authlib.oidc.core import CodeIDToken, IDToken
from ext_requests import organization_db, user_db

from users import sessions
from users.federated import KNOWN_ROLES, settings

logger = logging.getLogger("system_manager")

HTTP_TIMEOUT = 10
METADATA_TTL = 3600
JWKS_MIN_REFRESH = 60
LEEWAY = 60
RECENTLY_CONFIRMED = 30  # seconds: renewals this close to the last one reuse its answer
DEVICE_GRANT = "urn:ietf:params:oauth:grant-type:device_code"
SIGNING_ALGS = ["RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512"]
DEVICE_ERRORS = ("authorization_pending", "slow_down", "access_denied", "expired_token")
USERNAME_MAX = 64


class OidcError(Exception):
    """A sign-in that must stop. <code> is one of the codes the clients know (see the spec)."""

    def __init__(self, code, message, status=400):
        super().__init__(message)
        self.code = code
        self.message = message
        self.status = status


class ProviderUnavailable(Exception):
    """The provider could not be asked (network error, 5xx)."""


class ProviderRefused(Exception):
    """The provider answered and said no (invalid_grant, bad response, ...)."""

    def __init__(self, error, description=None):
        super().__init__(f"{error}: {description}" if description else error)
        self.error = error


# ............ provider metadata and keys ............ #

_cache = {"issuer": None, "metadata": None, "metadata_at": 0.0, "jwks": None, "jwks_at": 0.0}
_cache_lock = threading.Lock()


def reset_cache():
    with _cache_lock:
        _cache.update(issuer=None, metadata=None, metadata_at=0.0, jwks=None, jwks_at=0.0)


def _get_json(url, **kwargs):
    try:
        r = requests.get(url, timeout=HTTP_TIMEOUT, **kwargs)
    except requests.RequestException as e:
        raise ProviderUnavailable(f"{url}: {e}")
    if r.status_code >= 500:
        raise ProviderUnavailable(f"{url}: HTTP {r.status_code}")
    if r.status_code != 200:
        raise ProviderRefused("http_" + str(r.status_code), url)
    try:
        body = r.json()
    except ValueError:
        raise ProviderRefused("invalid_response", url)
    if not isinstance(body, dict):
        raise ProviderRefused("invalid_response", url)
    return body


def metadata():
    """The provider's discovery document, cached for an hour."""
    s = settings()
    with _cache_lock:
        fresh = time.monotonic() - _cache["metadata_at"] < METADATA_TTL
        if _cache["metadata"] and _cache["issuer"] == s.issuer and fresh:
            return _cache["metadata"]
    doc = _get_json(s.issuer.rstrip("/") + "/.well-known/openid-configuration")
    if str(doc.get("issuer", "")).rstrip("/") != s.issuer.rstrip("/"):
        raise ProviderRefused("invalid_metadata", "issuer in discovery does not match OIDC_ISSUER")
    for field in ("authorization_endpoint", "token_endpoint", "jwks_uri"):
        if not doc.get(field):
            raise ProviderRefused("invalid_metadata", f"discovery has no {field}")
    with _cache_lock:
        if _cache["issuer"] != s.issuer:
            _cache.update(jwks=None, jwks_at=0.0)
        _cache.update(issuer=s.issuer, metadata=doc, metadata_at=time.monotonic())
    return doc


def _jwks(force=False):
    meta = metadata()
    with _cache_lock:
        age = time.monotonic() - _cache["jwks_at"]
        if _cache["jwks"] is not None and (age < METADATA_TTL and not force):
            return _cache["jwks"]
        if _cache["jwks"] is not None and force and age < JWKS_MIN_REFRESH:
            return _cache["jwks"]
    try:
        keys = JsonWebKey.import_key_set(_get_json(meta["jwks_uri"]))
    except (ValueError, TypeError, KeyError) as e:
        raise ProviderRefused("invalid_jwks", str(e))
    with _cache_lock:
        _cache.update(jwks=keys, jwks_at=time.monotonic())
    return keys


def _algorithms():
    supported = metadata().get("id_token_signing_alg_values_supported") or ["RS256"]
    algs = [a for a in supported if a in SIGNING_ALGS]
    return algs or ["RS256"]


def validate_id_token(id_token, nonce=None, access_token=None, expected_sub=None):
    """Checks signature (provider JWKS), iss, aud/azp, exp/iat, nonce, at_hash; returns claims."""
    s = settings()
    options = {
        "iss": {"essential": True, "value": metadata()["issuer"]},
        "aud": {"essential": True, "value": s.client_id},
        "sub": {"essential": True},
    }
    if expected_sub is not None:
        options["sub"]["value"] = expected_sub
    params = {"client_id": s.client_id, "nonce": nonce, "access_token": access_token}
    jwt = JsonWebToken(_algorithms())
    claims_cls = CodeIDToken if nonce else IDToken
    for attempt in (False, True):  # retry once with fresh keys: the provider may have rotated
        try:
            claims = jwt.decode(
                id_token,
                key=_jwks(force=attempt),
                claims_cls=claims_cls,
                claims_options=options,
                claims_params=params,
            )
            break
        except ValueError as e:  # unknown kid
            if attempt:
                raise ProviderRefused("invalid_id_token", str(e))
        except AuthlibBaseError as e:
            if attempt or getattr(e, "error", "") != "bad_signature":
                raise ProviderRefused("invalid_id_token", str(e))
    try:
        claims.validate(leeway=LEEWAY)
    except AuthlibBaseError as e:
        raise ProviderRefused("invalid_id_token", str(e))
    return dict(claims)


# ............ OAuth client ............ #


def _check_token_response(resp):
    """Authlib compliance hook: tell outages apart from refusals."""
    if resp.status_code >= 500:
        raise ProviderUnavailable(f"token endpoint: HTTP {resp.status_code}")
    try:
        body = resp.json()
    except ValueError:
        raise ProviderRefused("invalid_response", f"token endpoint: HTTP {resp.status_code}")
    if resp.status_code != 200 and not (isinstance(body, dict) and body.get("error")):
        raise ProviderRefused("invalid_response", f"token endpoint: HTTP {resp.status_code}")
    return resp


def client(redirect_uri=None):
    s = settings()
    session = OAuth2Session(
        client_id=s.client_id,
        client_secret=s.client_secret,
        token_endpoint_auth_method="client_secret_basic",
        scope=s.scopes,
        redirect_uri=redirect_uri,
        code_challenge_method="S256",
        default_timeout=HTTP_TIMEOUT,
    )
    session.register_compliance_hook("access_token_response", _check_token_response)
    session.register_compliance_hook("refresh_token_response", _check_token_response)
    return session


def _token_call(fn, *args, **kwargs):
    try:
        token = fn(*args, **kwargs)
    except OAuthError as e:
        raise ProviderRefused(e.error or "invalid_grant", e.description)
    except requests.RequestException as e:
        raise ProviderUnavailable(str(e))
    if not isinstance(token, dict) or not token.get("access_token"):
        raise ProviderRefused("invalid_response", "token response without access_token")
    return dict(token)


def userinfo(access_token):
    endpoint = metadata().get("userinfo_endpoint")
    if not endpoint:
        return {}
    return _get_json(endpoint, headers={"Authorization": "Bearer " + access_token})


# ............ claims -> username and roles ............ #


def claim_path(claims, path):
    value = claims
    for part in path.split("."):
        if not isinstance(value, dict) or part not in value:
            return None
        value = value[part]
    return value


def map_roles(raw):
    """Provider roles -> Oakestra roles (after OIDC_ROLE_MAP), unknown names dropped."""
    if isinstance(raw, str):
        raw = raw.replace(",", " ").split()
    if not isinstance(raw, list):
        return []
    role_map = settings().role_map
    roles = set()
    for role in raw:
        if not isinstance(role, str):
            continue
        roles.update(role_map.get(role, {role}))
    return [r for r in KNOWN_ROLES if r in roles]


def identity_from(claims, access_token=None):
    """Username candidate, email and roles from the ID token, completed from userinfo if needed."""
    s = settings()
    raw_roles = claim_path(claims, s.roles_claim)
    if raw_roles is None and access_token:
        info = userinfo(access_token)
        if info.get("sub") not in (None, claims.get("sub")):
            raise ProviderRefused("invalid_userinfo", "userinfo sub differs from the ID token")
        claims = {**info, **claims}
        raw_roles = claim_path(info, s.roles_claim)
    username = None
    for name in (s.username_claim, "email", "sub"):
        value = claims.get(name)
        if isinstance(value, str) and value.strip():
            username = value.strip()
            break
    email = claims.get("email") if isinstance(claims.get("email"), str) else ""
    return {"username": username, "email": email, "roles": map_roles(raw_roles)}


def _clean_username(name):
    allowed = "".join(c if c.isalnum() or c in "._@+-" else "_" for c in name)
    return allowed.lstrip("._@+-")[:USERNAME_MAX] or "user"


def find_or_create_user(iss, sub, identity):
    """The Oakestra user for iss+sub, created on first sign-in. Local accounts are never taken."""
    user = mongodb.mongo_users.find_one({"oidc_iss": iss, "oidc_sub": sub})
    if user is not None:
        if identity["email"] and identity["email"] != user.get("email"):
            user_db.mongo_update_user(str(user["_id"]), {"email": identity["email"]})
        return user["name"], str(user["_id"])

    base = _clean_username(identity["username"] or sub)
    name = base
    for attempt in range(6):
        existing = user_db.mongo_get_user_by_name(name)
        if existing is None:
            break
        if not existing.get("oidc_iss") and not existing.get("federated"):
            raise OidcError("user_conflict", f"A local user named {name} already exists")
        suffix = sessions._hash(iss + "|" + sub)[:4] if attempt == 0 else secrets.token_hex(3)
        name = f"{base[: USERNAME_MAX - len(suffix) - 1]}-{suffix}"
    else:
        raise OidcError("user_conflict", f"No free username for {base}")

    user_id = user_db.mongo_save_user_without_roles(
        {
            "name": name,
            "email": identity["email"],
            "password": None,
            "federated": True,
            "oidc_iss": iss,
            "oidc_sub": sub,
            "created_at": sessions.now().strftime("%d/%m/%Y %H:%M"),
        }
    )
    logger.info("OIDC: created user %s for %s %s", name, iss, sub)
    return name, str(user_id)


def sync_roles(user_id, roles):
    """REPLACES the user's roles in the root organization; no roles removes the membership."""
    organization_id = sessions.root_organization_id()
    if roles:
        organization_db.mongo_add_user_role_to_organization(user_id, organization_id, roles)
    else:
        organization_db.mongo_delete_role_entry(user_id, organization_id)


def _sign_in(claims, token):
    """Shared tail of browser and device sign-in: user, roles, session, Oakestra tokens."""
    identity = identity_from(claims, token.get("access_token"))
    iss, sub = claims["iss"], claims["sub"]
    existing = mongodb.mongo_users.find_one({"oidc_iss": iss, "oidc_sub": sub})
    if not identity["roles"]:
        if existing is not None:
            sync_roles(str(existing["_id"]), [])
        raise OidcError("access_denied", "You have no Oakestra role on this cluster")
    username, user_id = find_or_create_user(iss, sub, identity)
    sync_roles(user_id, identity["roles"])
    sid = sessions.create_session(
        username,
        identity["roles"],
        iss=iss,
        sub=sub,
        provider_refresh_token=token.get("refresh_token"),
    )
    logger.info("OIDC: %s signed in with roles %s", username, identity["roles"])
    return username, identity["roles"], sid


# ............ browser login ............ #


def _host_of(url):
    try:
        return (urlsplit(url).hostname or "").lower()
    except ValueError:
        return ""


def allowed_return_to(return_to):
    if not isinstance(return_to, str) or not return_to:
        return False
    try:
        parts = urlsplit(return_to)
    except ValueError:
        return False
    if parts.scheme not in ("http", "https") or "@" in parts.netloc or not parts.hostname:
        return False
    s = settings()
    allowed = {_host_of(u) for u in s.redirect_uris} | set(s.allowed_return_hosts)
    return parts.hostname.lower() in allowed


def pick_redirect_uri(request_host):
    uris = settings().redirect_uris
    host = (request_host or "").lower()
    for uri in uris:
        if urlsplit(uri).netloc.lower() == host:
            return uri
    hostname = host.rsplit(":", 1)[0].strip("[]") if host else ""
    for uri in uris:
        if _host_of(uri) == hostname:
            return uri
    return uris[0]


def _require_configured():
    s = settings()
    if not s.issuer:
        raise OidcError("invalid_request", "OIDC login is not enabled on this cluster", 404)
    if s.missing():
        logger.error("OIDC login misconfigured, missing %s", s.missing())
        raise OidcError("provider_error", "OIDC login is misconfigured on this cluster", 500)


def login_url(return_to, request_host):
    """Starts a browser sign-in; returns the provider URL to redirect to."""
    _require_configured()
    if not allowed_return_to(return_to):
        raise OidcError("invalid_request", "return_to is not an allowed URL")
    try:
        meta = metadata()
    except (ProviderUnavailable, ProviderRefused) as e:
        logger.error("OIDC: cannot read provider metadata: %s", e)
        raise OidcError("provider_error", "The identity provider is not available", 502)
    redirect_uri = pick_redirect_uri(request_host)
    state = secrets.token_urlsafe(32)
    nonce = secrets.token_urlsafe(32)
    verifier = secrets.token_urlsafe(64)
    url, _ = client(redirect_uri).create_authorization_url(
        meta["authorization_endpoint"], state=state, code_verifier=verifier, nonce=nonce
    )
    sessions.save_state(
        state,
        {
            "nonce": nonce,
            "verifier": verifier,
            "return_to": return_to,
            "redirect_uri": redirect_uri,
        },
    )
    return url


def with_fragment(return_to, key, value):
    parts = urlsplit(return_to)
    return urlunsplit((parts.scheme, parts.netloc, parts.path, parts.query, f"{key}={value}"))


def callback(args):
    """Finishes a browser sign-in. Returns (return_to or None, fragment key, value)."""
    attempt = sessions.pop_state(args.get("state"))
    if attempt is None:
        logger.warning("OIDC callback with unknown or expired state")
        return None, "oidc_error", "invalid_request"
    return_to = attempt["return_to"]
    if args.get("error"):
        logger.warning(
            "OIDC provider returned %s: %s", args.get("error"), args.get("error_description")
        )
        code = "access_denied" if args.get("error") == "access_denied" else "provider_error"
        return return_to, "oidc_error", code
    if not args.get("code"):
        return return_to, "oidc_error", "invalid_request"
    try:
        _require_configured()
        token = _token_call(
            client(attempt["redirect_uri"]).fetch_token,
            metadata()["token_endpoint"],
            grant_type="authorization_code",
            code=args["code"],
            code_verifier=attempt["verifier"],
            redirect_uri=attempt["redirect_uri"],
        )
        if not token.get("id_token"):
            raise ProviderRefused("invalid_response", "no id_token")
        claims = validate_id_token(
            token["id_token"], nonce=attempt["nonce"], access_token=token["access_token"]
        )
        username, roles, sid = _sign_in(claims, token)
    except OidcError as e:
        logger.warning("OIDC sign-in refused: %s", e.message)
        return return_to, "oidc_error", e.code
    except (ProviderUnavailable, ProviderRefused) as e:
        logger.error("OIDC sign-in failed: %s", e)
        return return_to, "oidc_error", "provider_error"
    code = sessions.create_code({"username": username, "roles": roles, "sid": sid})
    return return_to, "oidc_code", code


def exchange(code):
    """One-time code -> Oakestra tokens. Raises OidcError when the code is unknown/used/expired."""
    doc = sessions.pop_code(code)
    if doc is None:
        raise OidcError("invalid_request", "Invalid or expired code")
    if doc.get("emergency"):
        minutes = doc["session_minutes"]
        sid = sessions.create_session(
            doc["username"],
            doc["roles"],
            emergency=True,
            expires_at=sessions.now() + timedelta(minutes=minutes),
        )
        logger.warning(
            "EMERGENCY login: %s Admin session for %d minutes (sid %s)",
            doc["username"],
            minutes,
            sid,
        )
        lifetime = timedelta(minutes=minutes)
        return sessions.issue_tokens(
            doc["username"], doc["roles"], sid, lifetime, lifetime, emergency=True
        )
    if sessions.get_session(doc.get("sid")) is None:  # signed out since the callback
        raise OidcError("invalid_request", "Invalid or expired code")
    return sessions.issue_tokens(doc["username"], doc["roles"], doc["sid"])


# ............ device login (CLI) ............ #


def device_start():
    _require_configured()
    try:
        endpoint = metadata().get("device_authorization_endpoint")
    except (ProviderUnavailable, ProviderRefused) as e:
        raise OidcError("provider_error", f"The identity provider is not available: {e}", 502)
    if not endpoint:
        raise OidcError(
            "invalid_request", "The identity provider does not support device login", 404
        )
    session = client()
    try:
        r = session.post(
            endpoint,
            data={"client_id": settings().client_id, "scope": settings().scopes},
            auth=session.client_auth("client_secret_basic"),
            withhold_token=True,
        )
        body = r.json()
    except requests.RequestException as e:
        raise OidcError("provider_error", f"The identity provider is not available: {e}", 502)
    except ValueError:
        raise OidcError("provider_error", "The identity provider sent an invalid answer", 502)
    if r.status_code != 200 or not isinstance(body, dict) or not body.get("device_code"):
        logger.error("OIDC device authorization failed: HTTP %s %s", r.status_code, body)
        raise OidcError("provider_error", "The identity provider refused device login", 502)
    verification_uri = body.get("verification_uri") or body.get("verification_url")
    return {
        "device_code": body["device_code"],
        "user_code": body.get("user_code"),
        "verification_uri": verification_uri,
        "verification_uri_complete": body.get("verification_uri_complete") or verification_uri,
        "expires_in": body.get("expires_in"),
        "interval": body.get("interval", 5),
    }


def device_token(device_code):
    """Polls the provider for a device login; returns Oakestra tokens once the user approved."""
    _require_configured()
    if not isinstance(device_code, str) or not device_code:
        raise OidcError("expired_token", "Missing device_code")
    try:
        token = _token_call(
            client().fetch_token,
            metadata()["token_endpoint"],
            grant_type=DEVICE_GRANT,
            device_code=device_code,
            client_id=settings().client_id,
        )
        if token.get("id_token"):
            claims = validate_id_token(token["id_token"], access_token=token["access_token"])
        else:
            info = userinfo(token["access_token"])
            if not info.get("sub"):
                raise ProviderRefused("invalid_response", "neither id_token nor userinfo sub")
            claims = {**info, "iss": metadata()["issuer"]}
        username, roles, sid = _sign_in(claims, token)
    except ProviderRefused as e:
        if e.error in DEVICE_ERRORS:
            raise OidcError(e.error, str(e))
        if e.error == "invalid_grant":
            raise OidcError("expired_token", "The device code is invalid or expired")
        logger.error("OIDC device login failed: %s", e)
        raise OidcError("access_denied", f"The identity provider refused the login: {e}")
    except ProviderUnavailable as e:
        raise OidcError("provider_error", f"The identity provider is not available: {e}", 502)
    except OidcError as e:
        if e.code in DEVICE_ERRORS or e.status != 400:
            raise
        raise OidcError("access_denied", e.message)  # e.g. user_conflict
    return sessions.issue_tokens(username, roles, sid)


# ............ renewal ............ #


class RenewalRefused(Exception):
    pass


def _end_session(session, reason):
    sessions.delete_session(session["_id"])
    logger.warning("OIDC session of %s ended: %s", session["username"], reason)
    raise RenewalRefused(reason)


def _issue(session, roles=None):
    return {
        "token": sessions.issue_access_token(
            session["username"], roles or session["roles"], session["_id"], settings().session
        )
    }


def renew(claims):
    """Access token for a federated session (POST /api/auth/refresh); RenewalRefused -> 401."""
    session = sessions.get_session(claims.get("sid"))
    if session is None or session["username"] != claims.get("sub"):
        raise RenewalRefused("session not found")

    if session.get("emergency"):
        remaining = sessions.aware(session["expires_at"]) - sessions.now()
        if remaining.total_seconds() <= 0:
            _end_session(session, "emergency session expired")
        return {
            "token": sessions.issue_access_token(
                session["username"], session["roles"], session["_id"], remaining, emergency=True
            )
        }

    confirmed = sessions.aware(session["confirmed_at"])
    if (sessions.now() - confirmed).total_seconds() < RECENTLY_CONFIRMED:
        return _issue(session)
    if not sessions.lock_session(session["_id"]):
        # Another renewal of the same session is asking the provider right now (e.g. two tabs);
        # don't send the rotated refresh token twice.
        if sessions.now() - confirmed > settings().grace:
            raise RenewalRefused("provider has not confirmed the user within the grace period")
        return _issue(session)
    try:
        return _renew_with_provider(session, confirmed)
    finally:
        sessions.unlock_session(session["_id"])


def _renew_with_provider(session, confirmed):
    provider_token = sessions.decrypt_token(session.get("refresh_token"), session["_id"])
    if not provider_token:
        _end_session(session, "no provider refresh token")
    try:
        token = _token_call(
            client().refresh_token, metadata()["token_endpoint"], refresh_token=provider_token
        )
        if token.get("id_token"):
            claims = validate_id_token(
                token["id_token"], access_token=token["access_token"], expected_sub=session["sub"]
            )
        else:
            claims = {"iss": session["iss"], "sub": session["sub"]}
        roles = identity_from(claims, token["access_token"])["roles"]
    except ProviderUnavailable as e:
        if sessions.now() - confirmed <= settings().grace:
            logger.warning(
                "OIDC provider unreachable (%s); renewing %s with its current roles",
                e,
                session["username"],
            )
            return _issue(session)
        _end_session(session, f"provider unreachable beyond the grace period ({e})")
    except ProviderRefused as e:
        _end_session(session, f"provider refused renewal ({e})")

    user = mongodb.mongo_users.find_one({"oidc_iss": session["iss"], "oidc_sub": session["sub"]})
    if user is not None:
        sync_roles(str(user["_id"]), roles)
    if not roles:
        _end_session(session, "no Oakestra role any more")
    fields = {"roles": roles, "confirmed_at": sessions.now()}
    if token.get("refresh_token") and token["refresh_token"] != provider_token:
        fields["refresh_token"] = sessions.encrypt_token(token["refresh_token"], session["_id"])
    sessions.update_session(session["_id"], fields)
    return _issue(session, roles)
