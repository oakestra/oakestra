import hashlib
import logging
import os
from datetime import datetime, timezone

from pymongo import MongoClient

logger = logging.getLogger("cluster_manager")

CLUSTER_MONGO_URL = os.environ.get("CLUSTER_MONGO_URL", "localhost")
CLUSTER_MONGO_PORT = os.environ.get("CLUSTER_MONGO_PORT", 10107)

_worker_tokens = None
_revoked_certs = None


def _collection():
    global _worker_tokens
    if _worker_tokens is None:
        client = MongoClient(f"mongodb://{CLUSTER_MONGO_URL}:{CLUSTER_MONGO_PORT}/")
        _worker_tokens = client["clusters"]["worker_tokens"]
        # Mongo TTL monitor garbage-collects expired one-time tokens.
        _worker_tokens.create_index("expiry_date", expireAfterSeconds=0)
    return _worker_tokens


def _revoked_certs_collection():
    global _revoked_certs
    if _revoked_certs is None:
        client = MongoClient(f"mongodb://{CLUSTER_MONGO_URL}:{CLUSTER_MONGO_PORT}/")
        _revoked_certs = client["clusters"]["revoked_certs"]
        # TTL: 730 days (twice max cert validity)
        _revoked_certs.create_index("revoked_at", expireAfterSeconds=63072000)
    return _revoked_certs


def normalize_serial_hex(serial_hex: str) -> str:
    return format(int(serial_hex.replace(":", ""), 16), "x")


def store_revoked_cert(
    serial_hex: str, cert_subject: str, not_after: datetime, reason: str, revoked_by: str
) -> None:
    """Record a revoked worker certificate; revoking the same serial again keeps the first record."""
    _revoked_certs_collection().update_one(
        {"serial_hex": serial_hex},
        {
            "$setOnInsert": {
                "cert_subject": cert_subject,
                "cert_type": "worker",
                "not_after": not_after,
                "revoked_at": datetime.now(timezone.utc),
                "reason": reason,
                "revoked_by": revoked_by,
            }
        },
        upsert=True,
    )


def get_revoked_serials() -> list:
    """Return list of (serial_int, revoked_at) tuples for CRL generation."""
    return [
        (int(doc["serial_hex"], 16), doc["revoked_at"])
        for doc in _revoked_certs_collection().find({}, {"serial_hex": 1, "revoked_at": 1})
    ]


def list_revoked_certs() -> list:
    docs = _revoked_certs_collection().find({}, {"_id": 0}).sort("revoked_at", -1)
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
    return _revoked_certs_collection().delete_one({"serial_hex": serial_hex}).deleted_count > 0


def clear_revoked_certs() -> int:
    return _revoked_certs_collection().delete_many({}).deleted_count


def prune_expired_revoked_certs() -> int:
    """Remove entries for certificates that have expired; those are rejected on expiry alone."""
    now = datetime.now(timezone.utc)
    return _revoked_certs_collection().delete_many({"not_after": {"$lte": now}}).deleted_count


def is_revoked(serial_hex: str) -> bool:
    return _revoked_certs_collection().count_documents({"serial_hex": serial_hex}, limit=1) > 0


_worker_renewals = None


def _worker_renewals_collection():
    global _worker_renewals
    if _worker_renewals is None:
        client = MongoClient(f"mongodb://{CLUSTER_MONGO_URL}:{CLUSTER_MONGO_PORT}/")
        _worker_renewals = client["clusters"]["worker_cert_renewals"]
        _worker_renewals.create_index("old_not_after", expireAfterSeconds=0)
    return _worker_renewals


def store_worker_renewal(
    new_serial_hex: str, old_serial_hex: str, old_subject: str, old_not_after: datetime
) -> None:
    """Remember old cert, to revoke it once the new one is in use."""
    _worker_renewals_collection().insert_one(
        {
            "new_serial_hex": new_serial_hex,
            "old_serial_hex": old_serial_hex,
            "old_subject": old_subject,
            "old_not_after": old_not_after,
        }
    )


def pop_worker_renewal(new_serial_hex: str):
    """Return and remove the renewal that issued new_serial_hex, or None."""
    return _worker_renewals_collection().find_one_and_delete({"new_serial_hex": new_serial_hex})


def hash_worker_token(token: str) -> str:
    # Same recipe as the root's registration tokens: only hashes are stored.
    return hashlib.pbkdf2_hmac("sha256", token.encode("ascii"), b"", 100000).hex()


def store_token_hash(token_hash: str, expiry_date: datetime) -> None:
    _collection().insert_one(
        {
            "token_hash": token_hash,
            "expiry_date": expiry_date,
            "created_at": datetime.now(timezone.utc),
        }
    )


def consume_token(token: str) -> dict:
    """Redeem a one-time worker token. Returns the document, or None if invalid.

    Delete-first semantics guarantee single use: even a token that turns out
    to be expired is removed on its first presentation.
    """
    if not token:
        return None
    doc = _collection().find_one_and_delete({"token_hash": hash_worker_token(token)})
    if doc is None:
        return None
    expiry_date = doc["expiry_date"]
    if expiry_date.tzinfo is None:
        expiry_date = expiry_date.replace(tzinfo=timezone.utc)
    if datetime.now(timezone.utc) >= expiry_date:
        logger.info("Rejected expired worker registration token")
        return None
    return doc
