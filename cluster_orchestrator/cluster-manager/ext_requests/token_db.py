import hashlib
import logging
import os
from datetime import datetime, timezone

from pymongo import MongoClient

logger = logging.getLogger("cluster_manager")

CLUSTER_MONGO_URL = os.environ.get("CLUSTER_MONGO_URL", "localhost")
CLUSTER_MONGO_PORT = os.environ.get("CLUSTER_MONGO_PORT", 10107)

_worker_tokens = None


def _collection():
    global _worker_tokens
    if _worker_tokens is None:
        client = MongoClient(f"mongodb://{CLUSTER_MONGO_URL}:{CLUSTER_MONGO_PORT}/")
        _worker_tokens = client["clusters"]["worker_tokens"]
        # Mongo TTL monitor garbage-collects expired one-time tokens.
        _worker_tokens.create_index("expiry_date", expireAfterSeconds=0)
    return _worker_tokens


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
