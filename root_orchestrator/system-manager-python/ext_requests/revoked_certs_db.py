from datetime import datetime, timezone

import ext_requests.mongodb_client as db


def normalize_serial_hex(serial_hex: str) -> str:
    # Accept "0A:1B", "0x0a1b" or upper case; records store format(serial, "x").
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
