"""
Server-side state of federated (OIDC) and emergency sessions, kept in the system manager's "users"
Mongo database next to the user and organization collections:

  oidc_state     login attempts (state, nonce, PKCE verifier, return_to), single use, 10 minutes
  oidc_code      one-time codes handed to the browser/CLI and exchanged for Oakestra tokens
  oidc_session   one per OIDC sign-in: user, roles, encrypted provider refresh token, last
                 time the provider confirmed the user
  auth_revocation  "revoked at" timestamps (per user and global) checked for every JWT

The provider refresh token is encrypted with AES-GCM (the session id is the associated data, so a
ciphertext cannot be moved to another session). The key comes from OIDC_TOKEN_ENCRYPTION_KEY if set,
otherwise it is derived with HKDF from OIDC_CLIENT_SECRET. The JWT key pair is not an option: the
system manager only holds the public key (the private key lives in the jwt_generator container and
is regenerated on every start unless pinned). Both secrets live in the environment, not in Mongo, so
a database dump alone does not reveal provider tokens. If the key changes (e.g. the client secret is
rotated), stored tokens no longer decrypt and those users simply have to sign in again.
"""

import base64
import hashlib
import logging
import os
import secrets
import threading
import time
from datetime import datetime, timedelta, timezone

import ext_requests.mongodb_client as db
import pymongo
from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from ext_requests import organization_db
from roles import securityUtils

from users.federated import settings

logger = logging.getLogger("system_manager")

STATE_TTL = timedelta(minutes=10)
CODE_TTL = timedelta(seconds=60)
EMERGENCY_CODE_TTL = timedelta(seconds=600)
REVOCATION_CACHE_SECONDS = 5

STATES = "oidc_state"
CODES = "oidc_code"
SESSIONS = "oidc_session"
REVOCATIONS = "auth_revocation"
GLOBAL_REVOCATION = "*"


def now():
    return datetime.now(timezone.utc)


def aware(value):
    """Mongo returns naive UTC datetimes; make them comparable with now()."""
    if value is not None and value.tzinfo is None:
        return value.replace(tzinfo=timezone.utc)
    return value


def collection(name):
    return db.mongo_users.database[name]


def ensure_indexes():
    try:
        for name in (STATES, CODES):
            collection(name).create_index("expires_at", expireAfterSeconds=0)
        collection(SESSIONS).create_index([("username", pymongo.ASCENDING)])
        db.mongo_users.create_index(
            [("oidc_iss", pymongo.ASCENDING), ("oidc_sub", pymongo.ASCENDING)]
        )
    except Exception as e:  # indexes are an optimisation, expiry is also checked in code
        logger.warning("Could not create OIDC indexes: %s", e)


def _hash(value):
    return hashlib.sha256(value.encode()).hexdigest()


# ............ login state and one-time codes ............ #


def save_state(state, data):
    collection(STATES).insert_one({"_id": _hash(state), **data, "expires_at": now() + STATE_TTL})


def pop_state(state):
    """Returns the login attempt for <state> once; None when unknown, used or expired."""
    if not isinstance(state, str) or not state:
        return None
    doc = collection(STATES).find_one_and_delete({"_id": _hash(state)})
    if doc is None or aware(doc["expires_at"]) < now():
        return None
    return doc


def create_code(payload, ttl=CODE_TTL):
    code = secrets.token_urlsafe(32)
    collection(CODES).insert_one({"_id": _hash(code), **payload, "expires_at": now() + ttl})
    return code


def pop_code(code):
    if not isinstance(code, str) or not code:
        return None
    doc = collection(CODES).find_one_and_delete({"_id": _hash(code)})
    if doc is None or aware(doc["expires_at"]) < now():
        return None
    return doc


# ............ provider refresh token encryption ............ #


def _key():
    s = settings()
    if s.token_encryption_key:
        material = s.token_encryption_key.encode()
    elif s.client_secret:
        material = s.client_secret.encode()
    else:
        raise RuntimeError("No key material for provider tokens (set OIDC_CLIENT_SECRET)")
    return HKDF(
        algorithm=hashes.SHA256(),
        length=32,
        salt=b"oakestra-system-manager",
        info=b"oidc-provider-refresh-token",
    ).derive(material)


def encrypt_token(token, sid):
    if not token:
        return None
    nonce = os.urandom(12)
    sealed = AESGCM(_key()).encrypt(nonce, token.encode(), sid.encode())
    return base64.urlsafe_b64encode(nonce + sealed).decode()


def decrypt_token(blob, sid):
    """Returns the provider token, or None if missing or no longer decryptable."""
    if not blob:
        return None
    try:
        raw = base64.urlsafe_b64decode(blob.encode())
        return AESGCM(_key()).decrypt(raw[:12], raw[12:], sid.encode()).decode()
    except (InvalidTag, ValueError, RuntimeError) as e:
        logger.warning("Stored provider token of session %s cannot be decrypted: %s", sid, e)
        return None


# ............ sessions ............ #


def create_session(username, roles, iss=None, sub=None, provider_refresh_token=None, **extra):
    sid = secrets.token_urlsafe(24)
    collection(SESSIONS).insert_one(
        {
            "_id": sid,
            "username": username,
            "roles": roles,
            "iss": iss,
            "sub": sub,
            "refresh_token": encrypt_token(provider_refresh_token, sid),
            "confirmed_at": now(),
            "created_at": now(),
            **extra,
        }
    )
    return sid


def get_session(sid):
    if not isinstance(sid, str) or not sid:
        return None
    return collection(SESSIONS).find_one({"_id": sid})


def update_session(sid, fields):
    collection(SESSIONS).update_one({"_id": sid}, {"$set": fields})


def lock_session(sid, seconds=20):
    """Marks the session as being renewed; False if another renewal holds the lock."""
    doc = collection(SESSIONS).find_one_and_update(
        {"_id": sid, "$or": [{"renewing_until": None}, {"renewing_until": {"$lt": now()}}]},
        {"$set": {"renewing_until": now() + timedelta(seconds=seconds)}},
    )
    return doc is not None


def unlock_session(sid):
    collection(SESSIONS).update_one({"_id": sid}, {"$set": {"renewing_until": None}})


def delete_session(sid):
    collection(SESSIONS).delete_one({"_id": sid})


def delete_sessions_of(username, before=None):
    query = {"username": username}
    if before is not None:
        query["confirmed_at"] = {"$lt": before}
    return collection(SESSIONS).delete_many(query).deleted_count


# ............ Oakestra tokens ............ #


def root_organization_id():
    return str(organization_db.mongo_get_organization_by_name("root")["_id"])


def session_claims(username, roles, sid, emergency=False):
    claims = {
        "user": username,
        "roles": roles,
        "organization": root_organization_id(),
        "federated": True,
        "sid": sid,
    }
    if emergency:
        claims["emergency"] = True
    return claims


def issue_access_token(username, roles, sid, expires, emergency=False):
    return securityUtils.create_jwt_auth_access_token(
        identity=username,
        additional_claims=session_claims(username, roles, sid, emergency),
        expires_delta=expires,
    )


def issue_tokens(username, roles, sid, expires=None, refresh_expires=None, emergency=False):
    expires = expires or settings().session
    return {
        "token": issue_access_token(username, roles, sid, expires, emergency),
        "refresh_token": securityUtils.create_jwt_auth_refresh_token(
            identity=username,
            additional_claims=session_claims(username, roles, sid, emergency),
            expires_delta=refresh_expires,
        ),
    }


# ............ sign-out (JWT blocklist) ............ #

_revocations = {"loaded": 0.0, "global": None, "users": {}}
_revocations_lock = threading.Lock()


def _invalidate_revocation_cache():
    with _revocations_lock:
        _revocations["loaded"] = 0.0


def _revocation_snapshot():
    with _revocations_lock:
        if time.monotonic() - _revocations["loaded"] < REVOCATION_CACHE_SECONDS:
            return _revocations["global"], _revocations["users"]
        try:
            docs = list(collection(REVOCATIONS).find())
        except Exception as e:  # keep serving with the last known list
            logger.error("Cannot load sign-outs: %s", e)
            return _revocations["global"], _revocations["users"]
        users = {}
        global_at = None
        for d in docs:
            at = aware(d["at"]).timestamp()
            if d["_id"] == GLOBAL_REVOCATION:
                global_at = at
            else:
                users[d["_id"]] = at
        _revocations.update(loaded=time.monotonic(), users=users)
        _revocations["global"] = global_at
        return global_at, users


def token_revoked(jwt_header, jwt_payload):
    """flask_jwt_extended token_in_blocklist_loader: rejects tokens issued before a sign-out."""
    iat = jwt_payload.get("iat")
    if not isinstance(iat, (int, float)):
        return False
    global_at, users = _revocation_snapshot()
    if global_at is not None and iat < global_at:
        return True
    user_at = users.get(jwt_payload.get("sub") or jwt_payload.get("user"))
    return user_at is not None and iat < user_at


def _revoke(key, at):
    collection(REVOCATIONS).update_one({"_id": key}, {"$max": {"at": at}}, upsert=True)
    _invalidate_revocation_cache()


def signout_user(username, at=None):
    """Rejects the user's existing tokens and drops their stored provider refresh tokens."""
    at = at or now()
    _revoke(username, at)
    dropped = delete_sessions_of(username, before=at)
    logger.warning("Signed out %s (%d sessions dropped)", username, dropped)


def signout_all():
    at = now()
    _revoke(GLOBAL_REVOCATION, at)
    dropped = collection(SESSIONS).delete_many({}).deleted_count
    logger.warning("Signed out everyone (%d sessions dropped)", dropped)
