import hashlib
import logging
import os
import secrets
from datetime import datetime, timedelta, timezone

import ext_requests.mongodb_client as db

logger = logging.getLogger("system_manager")

DEFAULT_TOKEN_TTL_MINUTES = int(os.environ.get("REGISTRATION_TOKEN_TTL_MINUTES") or 10)

TOKEN_TYPE_CLUSTER = "cluster"


# ---------------------------------------------------------------------------
# Registration tokens
# ---------------------------------------------------------------------------


def hash_registration_token(token: str) -> str:
    # Only the hash is ever stored, so a database leak does not leak usable tokens.
    return hashlib.pbkdf2_hmac("sha256", token.encode("ascii"), b"", 100000).hex()


def generate_token() -> tuple[str, str, datetime]:
    """Generate a one-time token without persisting it.

    Returns (token, token_hash, expiry_date). Used for worker tokens, whose
    hash is pushed to the target cluster instead of being stored at the root.
    """
    token = secrets.token_urlsafe(32)
    expiry_date = datetime.now(timezone.utc) + timedelta(minutes=DEFAULT_TOKEN_TTL_MINUTES)
    return token, hash_registration_token(token), expiry_date


def create_registration_token(token_type: str, created_by: str) -> tuple[str, datetime]:
    """Create and persist a one-time registration token. Returns (token, expiry_date)."""
    token, token_hash, expiry_date = generate_token()
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


# ---------------------------------------------------------------------------
# Revoked certificates
# ---------------------------------------------------------------------------


def normalize_serial_hex(serial_hex: str) -> str:
    return format(int(serial_hex.replace(":", ""), 16), "x")


def add_revoked_cert(
    serial_hex: str, cert_subject: str, not_after: datetime, reason: str, revoked_by: str
) -> None:
    """Record a revoked cluster certificate; revoking the same serial again keeps the first record."""
    db.mongo_revoked_certs.update_one(
        {"serial_hex": serial_hex},
        {
            "$setOnInsert": {
                "cert_subject": cert_subject,
                "cert_type": "cluster",
                "not_after": not_after,
                "revoked_at": datetime.now(timezone.utc),
                "reason": reason,
                "revoked_by": revoked_by,
            }
        },
        upsert=True,
    )


def get_revoked_serials() -> list:
    """Return (serial_int, revoked_at) tuples for CRL generation."""
    return [
        (int(doc["serial_hex"], 16), doc["revoked_at"])
        for doc in db.mongo_revoked_certs.find({}, {"serial_hex": 1, "revoked_at": 1})
    ]


def list_revoked_certs() -> list:
    docs = db.mongo_revoked_certs.find({}, {"_id": 0}).sort("revoked_at", -1)
    return [
        {
            key: value.isoformat() if isinstance(value, datetime) else value
            for key, value in doc.items()
        }
        for doc in docs
    ]


def remove_revoked_cert(serial_hex: str) -> bool:
    try:
        serial_hex = normalize_serial_hex(serial_hex)
    except ValueError:
        return False
    return db.mongo_revoked_certs.delete_one({"serial_hex": serial_hex}).deleted_count > 0


def clear_revoked_certs() -> int:
    return db.mongo_revoked_certs.delete_many({}).deleted_count


def prune_expired_revoked_certs() -> int:
    """Remove entries for certificates that have expired; those are rejected on expiry alone."""
    now = datetime.now(timezone.utc)
    return db.mongo_revoked_certs.delete_many({"not_after": {"$lte": now}}).deleted_count
