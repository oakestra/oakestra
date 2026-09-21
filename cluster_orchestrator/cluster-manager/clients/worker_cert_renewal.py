"""Worker certificate renewal, driven by the worker heartbeat.

Workers report their certificate's serial, expiry and issuing intermediate with every
heartbeat. The cluster asks a worker to renew (it then calls /api/certs/worker-renew
over mTLS) when its cert is close to expiry or was issued by a replaced intermediate,
and revokes the old cert once the heartbeat shows the renewed one in use.
"""

import json
import logging
import time
from datetime import datetime, timedelta, timezone

import config
from ext_requests.cluster_certificates import cluster_ca_key_ids
from ext_requests.token_db import pop_worker_renewal, store_revoked_cert

logger = logging.getLogger("cluster_manager")

# Heartbeats arrive every few seconds; ask a worker to renew at most this often.
RENEW_COMMAND_INTERVAL_SECONDS = 600

_last_renew_command = {}
# Serials already matched against pending renewals, so each costs one database lookup.
_seen_serials = set()


def check_worker_cert(node_id: str, cert_info: dict, publish) -> None:
    serial = cert_info.get("serial") or ""
    if serial and serial not in _seen_serials:
        _revoke_replaced_cert(serial)
        _seen_serials.add(serial)

    current_id, _ = cluster_ca_key_ids()
    expires = datetime.fromtimestamp(int(cert_info.get("not_after") or 0), timezone.utc)
    expiring = expires - datetime.now(timezone.utc) <= timedelta(days=config.WORKER_CERT_RENEW_DAYS)
    if cert_info.get("issuer_key_id") == current_id and not expiring:
        return

    now = time.monotonic()
    if now - _last_renew_command.get(node_id, float("-inf")) < RENEW_COMMAND_INTERVAL_SECONDS:
        return
    _last_renew_command[node_id] = now
    logger.info(
        "Asking worker %s to renew its certificate (%s)",
        node_id,
        "expires " + expires.isoformat() if expiring else "issued by a replaced intermediate",
    )
    publish(f"nodes/{node_id}/control/renew-cert", json.dumps({}))


def _revoke_replaced_cert(new_serial: str) -> None:
    renewal = pop_worker_renewal(new_serial)
    if renewal is None:
        return
    from blueprints.certificates_blueprints import refresh_cluster_crl

    store_revoked_cert(
        serial_hex=renewal["old_serial_hex"],
        cert_subject=renewal["old_subject"],
        not_after=renewal["old_not_after"],
        reason="superseded",
        revoked_by="renewal",
    )
    refresh_cluster_crl()
    logger.info(
        "Revoked certificate %s of %s: the worker now uses %s",
        renewal["old_serial_hex"],
        renewal["old_subject"],
        new_serial,
    )
