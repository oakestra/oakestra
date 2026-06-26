import hashlib
import logging
import os
import secrets
from datetime import datetime, timedelta, timezone

import ext_requests.mongodb_client as db

logger = logging.getLogger("system_manager")

DEFAULT_TOKEN_TTL_MINUTES = int(os.environ.get("REGISTRATION_TOKEN_TTL_MINUTES") or 10)

TOKEN_TYPE_CLUSTER = "cluster"


def hash_registration_token(token: str) -> str:
    # Same recipe as the password-reset tokens (users/auth.py): only the hash
    # is ever stored, so a database leak does not leak usable tokens.
    return hashlib.pbkdf2_hmac("sha256", token.encode("ascii"), b"", 100000).hex()


def generate_token(ttl_minutes: int = None) -> tuple[str, str, datetime]:
    """Generate a one-time token without persisting it.

    Returns (token, token_hash, expiry_date). Used for worker tokens, whose
    hash is pushed to the target cluster instead of being stored at the root.
    """
    ttl = int(ttl_minutes or DEFAULT_TOKEN_TTL_MINUTES)
    token = secrets.token_urlsafe(32)
    expiry_date = datetime.now(timezone.utc) + timedelta(minutes=ttl)
    return token, hash_registration_token(token), expiry_date


def create_registration_token(
    token_type: str, created_by: str, ttl_minutes: int = None
) -> tuple[str, datetime]:
    """Create and persist a one-time registration token. Returns (token, expiry_date)."""
    token, token_hash, expiry_date = generate_token(ttl_minutes)
    db.mongo_registration_tokens.insert_one(
        {
            "token_hash": token_hash,
            "type": token_type,
            "created_by": created_by,
            "created_at": datetime.now(timezone.utc),
            "expiry_date": expiry_date,
        }
    )
    return token, expiry_date


def consume_registration_token(token: str, token_type: str) -> dict:
    """Redeem a one-time token. Returns the token document, or None if invalid.

    Delete-first semantics guarantee single use: even a token that turns out
    to be expired is removed on its first presentation.
    """
    if not token:
        return None
    doc = db.mongo_registration_tokens.find_one_and_delete(
        {"token_hash": hash_registration_token(token), "type": token_type}
    )
    if doc is None:
        return None
    expiry_date = doc["expiry_date"]
    if expiry_date.tzinfo is None:
        expiry_date = expiry_date.replace(tzinfo=timezone.utc)
    if datetime.now(timezone.utc) >= expiry_date:
        logger.info("Rejected expired %s registration token", token_type)
        return None
    return doc
