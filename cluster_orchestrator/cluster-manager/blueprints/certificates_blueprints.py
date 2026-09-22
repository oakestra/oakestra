import logging
import os
import threading
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path
from urllib.parse import unquote

import config
import requests
from cryptography import x509
from cryptography.x509.oid import NameOID
from ext_requests.certificates_db import (
    clear_revoked_certs,
    consume_token,
    get_revoked_serials,
    is_revoked,
    list_revoked_certs,
    prune_expired_revoked_certs,
    remove_revoked_cert,
    store_revoked_cert,
    store_token_hash,
    store_worker_renewal,
)
from flask import Response, request
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from utils.certificates import (
    csr_common_name,
    generate_cluster_csr,
    generate_worker_cert,
    get_old_cluster_ca_paths,
    get_root_ca_pem,
    issued_by_cluster_ca,
    load_worker_csr,
    regenerate_cluster_crl,
    sign_worker_csr,
    write_cluster_mqtt_identity,
    write_mqtt_server_identity,
)

logger = logging.getLogger("cluster_manager")

certbp = Blueprint(
    "Cluster certificates",
    "cluster_certificates",
    url_prefix="/api/certs",
    description="Worker certificate provisioning (internal gateway only)",
)


generate_worker_schema = {
    "type": "object",
    "properties": {
        "common_name": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["common_name"],
}

sign_csr_schema = {
    "type": "object",
    "properties": {
        "csr": {"type": "string"},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["csr"],
}

worker_token_schema = {
    "type": "object",
    "properties": {
        "token_hash": {"type": "string"},
        "expiry_date": {"type": "string"},
    },
    "required": ["token_hash", "expiry_date"],
}

worker_bootstrap_schema = {
    "type": "object",
    "properties": {
        "token": {"type": "string"},
        "csr": {"type": "string"},
    },
    "required": ["token", "csr"],
}

worker_renew_schema = {
    "type": "object",
    "properties": {
        "csr": {"type": "string"},
    },
    "required": ["csr"],
}

cluster_refresh_schema = {
    "type": "object",
    "properties": {
        "token": {"type": "string"},
    },
    "required": ["token"],
}

revoke_cert_schema = {
    "type": "object",
    "properties": {
        "cert_pem": {"type": "string"},
        "reason": {"type": "string"},
    },
    "required": ["cert_pem"],
}


@certbp.route("/ca.crt")
class ClusterRootCaController(MethodView):
    def get(self):
        try:
            pem = get_root_ca_pem()
        except FileNotFoundError:
            abort(503, message="Root CA not available on this cluster")
        return Response(
            pem,
            mimetype="application/x-pem-file",
            headers={"Content-Disposition": "attachment; filename=ca.crt"},
        )


@certbp.route("/generate-worker")
class ClusterGenerateWorkerController(MethodView):
    @certbp.arguments(schema=generate_worker_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        common_name = content.get("common_name")
        if not common_name:
            abort(400, message="common_name is required")

        alt_names = content.get("alt_names") or None
        valid_days = int(content.get("valid_days") or 365)

        try:
            private_pem, fullchain_pem = generate_worker_cert(
                common_name=common_name, alt_names=alt_names, valid_days=valid_days
            )
            root_ca_pem = get_root_ca_pem()
        except FileNotFoundError:
            abort(409, message="Cluster intermediate CA material is not initialized")
        except ValueError as e:
            abort(400, message=f"Worker certificate generation failed: {str(e)}")

        return {
            "private_key": private_pem,
            "certificate": fullchain_pem,
            "root_ca": root_ca_pem,
        }


@certbp.route("/worker-token")
class ClusterWorkerTokenController(MethodView):
    @certbp.arguments(schema=worker_token_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Receive a one-time worker registration token hash from the root.

        This hash is used to validate incoming worker registration attemps.
        """
        content = request.get_json(silent=True) or {}
        token_hash = content.get("token_hash") or ""
        expiry_raw = content.get("expiry_date") or ""
        if not token_hash.strip():
            abort(400, message="token_hash is required")

        try:
            expiry_date = datetime.fromisoformat(expiry_raw)
        except ValueError:
            abort(400, message="expiry_date must be an ISO-8601 timestamp")
        if expiry_date.tzinfo is None:
            expiry_date = expiry_date.replace(tzinfo=timezone.utc)

        store_token_hash(token_hash, expiry_date)
        logger.info("Stored worker registration token (expires %s)", expiry_date.isoformat())
        return "", 204


@certbp.route("/worker-bootstrap")
class ClusterWorkerBootstrapController(MethodView):
    @certbp.arguments(schema=worker_bootstrap_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Redeem a one-time worker token for a worker certificate.

        The worker sends a CSR, so its private key never leaves the worker. Returns the
        certificate chain and the CA bundle used to verify the cluster.
        """
        content = request.get_json(silent=True) or {}
        # Check the CSR before consuming the token, so a bad request does not burn it.
        try:
            common_name = csr_common_name(load_worker_csr(content.get("csr") or ""))
        except ValueError as e:
            abort(400, message=str(e))

        if consume_token(content.get("token") or "") is None:
            abort(401, message="invalid registration token")

        try:
            fullchain_pem = sign_worker_csr(content["csr"])
            root_ca_pem = get_root_ca_pem()
        except FileNotFoundError:
            abort(409, message="Cluster intermediate CA material is not initialized")
        except ValueError as e:
            abort(400, message=f"Worker certificate generation failed: {str(e)}")

        logger.info(f"Worker bootstrap: issued certificate for '{common_name}'")
        return {"certificate": fullchain_pem, "root_ca": root_ca_pem}


@certbp.route("/worker-renew")
class ClusterWorkerRenewController(MethodView):
    @certbp.arguments(schema=worker_renew_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Renew a worker certificate, authenticated by the worker's current certificate.

        The new certificate keeps the presented certificate's name; the presented one is revoked
        once the worker's heartbeat shows the new one in use.
        """
        try:
            presented = x509.load_pem_x509_certificate(
                unquote(request.headers.get("X-Client-Cert", "")).encode("utf-8")
            )
        except ValueError:
            abort(401, message="Client certificate required")
        # The gateway only checks the chain up to the root CA; this cluster must have issued it.
        if not issued_by_cluster_ca(presented):
            abort(401, message="Client certificate was not issued by this cluster")
        serial_hex = format(presented.serial_number, "x")
        if is_revoked(serial_hex):
            abort(401, message="Client certificate is revoked")
        presented_names = presented.subject.get_attributes_for_oid(NameOID.COMMON_NAME)

        content = request.get_json(silent=True) or {}
        try:
            csr = load_worker_csr(content.get("csr") or "")
            if not presented_names or csr_common_name(csr) != presented_names[0].value:
                abort(403, message="CSR name must match the client certificate")
            fullchain_pem = sign_worker_csr(content["csr"])
            root_ca_pem = get_root_ca_pem()
        except FileNotFoundError:
            abort(409, message="Cluster intermediate CA material is not initialized")
        except ValueError as e:
            abort(400, message=f"Worker certificate renewal failed: {str(e)}")

        new_cert = x509.load_pem_x509_certificate(fullchain_pem.encode("utf-8"))
        store_worker_renewal(
            new_serial_hex=format(new_cert.serial_number, "x"),
            old_serial_hex=serial_hex,
            old_subject=presented.subject.rfc4514_string(),
            old_not_after=presented.not_valid_after_utc,
        )
        logger.info("Worker renewal: issued certificate for '%s'", presented_names[0].value)
        return {"certificate": fullchain_pem, "root_ca": root_ca_pem}


@certbp.route("/refresh")
class ClusterCertRefreshController(MethodView):
    @certbp.arguments(schema=cluster_refresh_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Re-bootstrap the cluster's mTLS certificate material from the root.

        Accepts a one-time cluster registration token. Rewrites all mTLS cluster cert
        files in the mounted cert directory.
        """
        content = request.get_json(silent=True) or {}
        token = content.get("token") or ""
        if not token.strip():
            abort(400, message="token is required")

        root_url = os.environ.get("SYSTEM_MANAGER_URL") or ""
        root_port = os.environ.get("SYSTEM_MANAGER_PORT") or "443"
        cluster_name = config.MY_CHOSEN_CLUSTER_NAME or ""
        cluster_ip = config.MY_CLUSTER_ADDRESS or ""

        if not root_url or not cluster_name:
            abort(500, message="SYSTEM_MANAGER_URL and CLUSTER_NAME must be configured")

        base_url = f"https://{root_url}:{root_port}"
        alt_names = [name for name in (cluster_ip,) if name]

        try:
            resp = requests.post(
                f"{base_url}/api/certs/cluster-bootstrap",
                json={"token": token, "cluster_name": cluster_name, "alt_names": alt_names},
                verify=config.root_gateway_verify(),
                timeout=30,
            )
        except requests.exceptions.RequestException as e:
            abort(502, message=f"Could not reach root: {e}")

        if resp.status_code == 401:
            abort(401, message="Registration token rejected (invalid, expired, or already used)")
        if resp.status_code != 200:
            abort(
                502,
                message=f"Root bootstrap endpoint returned {resp.status_code}: {resp.text[:200]}",
            )

        payload = resp.json()

        cert_dir = Path(config.ROOT_CA_FILE).parent if config.ROOT_CA_FILE else Path("/certs")

        def _write(path, content, mode):
            p = Path(path)
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(content)
            os.chmod(p, mode)

        _write(config.ROOT_CA_FILE, payload["root_ca"], 0o644)
        _write(config.CLUSTER_CERT_FILE, payload["client_cert"], 0o644)
        _write(config.CLUSTER_KEY_FILE, payload["client_key"], 0o600)
        _write(config.CLUSTER_CA_CERT_FILE, payload["cluster_ca_cert"], 0o644)
        _write(config.CLUSTER_CA_KEY_FILE, payload["cluster_ca_key"], 0o600)

        logger.info("Cluster certificate material refreshed from root")
        # The MQTT identities and CRL must chain to / be signed by the new cluster CA.
        write_cluster_mqtt_identity(cluster_name)
        write_mqtt_server_identity(cluster_name, cluster_ip)
        mqtt_reloaded = refresh_cluster_crl()["mqtt_reloaded"]
        kong_reloaded = config.reload_kong_external()
        return {
            "message": (
                "Certificates refreshed and MQTT broker reloaded."
                if mqtt_reloaded
                else "Certificates refreshed. MQTT broker reload failed — restart mqtt manually."
            ),
            "mqtt_reloaded": mqtt_reloaded,
            "kong_reloaded": kong_reloaded,
            "cert_dir": str(cert_dir),
        }


def _renew_cluster_certs_in_band(
    rotate_intermediate: bool = False, grace_hours: float | None = None
) -> tuple:
    """Renew cluster mTLS certs using current certs as authentication secret.

    grace_hours sets how long workers may migrate off a replaced intermediate; None uses
    INTERMEDIATE_GRACE_PERIOD_HOURS. Returns (success: bool, message: str).
    """
    # Expiry checks, rotation detection and manual calls can all trigger a renewal.
    if not _renew_lock.acquire(blocking=False):
        return False, "A certificate renewal is already in progress"
    try:
        return _renew_cluster_certs(rotate_intermediate, grace_hours)
    finally:
        _renew_lock.release()


_renew_lock = threading.Lock()


def _renew_cluster_certs(rotate_intermediate: bool, grace_hours: float | None = None) -> tuple:
    root_url = os.environ.get("SYSTEM_MANAGER_URL") or ""
    root_port = os.environ.get("SYSTEM_MANAGER_PORT") or "443"
    cluster_name = config.MY_CHOSEN_CLUSTER_NAME or ""
    cluster_ip = config.MY_CLUSTER_ADDRESS or ""

    if not root_url or not cluster_name:
        return False, "SYSTEM_MANAGER_URL and CLUSTER_NAME must be configured"
    if not config.mtls_enabled():
        return False, "mTLS is not enabled — cannot renew in-band"

    alt_names = [n for n in (cluster_ip, cluster_name, "mqtt", "localhost") if n]
    new_key_pem, csr_pem = generate_cluster_csr(cluster_name, alt_names)

    base_url = f"https://{root_url}:{root_port}"
    try:
        resp = requests.post(
            f"{base_url}/api/certs/cluster-renew",
            json={
                "csr": csr_pem,
                "alt_names": alt_names,
                "valid_days": 365,
                "rotate_intermediate": rotate_intermediate,
            },
            cert=(config.CLUSTER_CERT_FILE, config.CLUSTER_KEY_FILE),
            verify=config.root_gateway_verify(),
            timeout=30,
        )
    except requests.exceptions.RequestException as e:
        return False, f"Could not reach root: {e}"

    if resp.status_code == 401:
        return False, "mTLS client cert rejected — grace period may have ended"
    if resp.status_code != 200:
        return False, f"Root returned {resp.status_code}: {resp.text[:200]}"

    try:
        payload = resp.json()
        new_files = [
            (config.CLUSTER_KEY_FILE, new_key_pem, 0o600),
            (config.CLUSTER_CERT_FILE, payload["client_cert"], 0o644),
            (config.ROOT_CA_FILE, payload["root_ca"], 0o644),
        ]
        if rotate_intermediate:
            new_files += [
                (config.CLUSTER_CA_CERT_FILE, payload["cluster_ca_cert"], 0o644),
                (config.CLUSTER_CA_KEY_FILE, payload["cluster_ca_key"], 0o600),
            ]
    except (ValueError, KeyError, TypeError) as e:
        return False, f"Root returned an invalid renewal response: {e!r}"
    cert_dir = Path(config.ROOT_CA_FILE).parent if config.ROOT_CA_FILE else Path("/certs")

    def _write(path, content, mode):
        p = Path(path)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content)
        os.chmod(p, mode)

    if rotate_intermediate:
        _keep_old_intermediate(grace_hours)
    for path, content, mode in new_files:
        _write(path, content, mode)

    logger.info("Cluster certificates renewed in-band in %s", cert_dir)
    if rotate_intermediate:
        # MQTT accepts both intermediates until the workers have migrated. The broker's server
        # cert moves when the migration ends.
        write_cluster_mqtt_identity(cluster_name)
    # Re-signs the CRL (with the new cluster CA if renewed) and reloads MQTT for the new root CA.
    mqtt_reloaded = refresh_cluster_crl()["mqtt_reloaded"]
    # The external gateway verifies the root's calls against ca.crt, which may have changed.
    kong_reloaded = config.reload_kong_external()
    msg = "Certificates renewed."
    if not mqtt_reloaded:
        msg += " MQTT broker reload failed — restart mqtt manually."
    if not kong_reloaded:
        msg += " External gateway reload failed — restart cluster_kong_external manually."
    return True, msg


def _keep_old_intermediate(grace_hours: float | None = None):
    """Keep the replaced intermediate while workers migrate to the new one.

    Until the deadline mosquitto accepts both intermediates, /worker-renew accepts certs
    from either, and worker heartbeats trigger renewals onto the new one.
    """
    if grace_hours is None:
        grace_hours = config.INTERMEDIATE_GRACE_PERIOD_HOURS
    cert_path, key_path, expiry_path = get_old_cluster_ca_paths()
    cert_path.write_text(Path(config.CLUSTER_CA_CERT_FILE).read_text())
    os.chmod(cert_path, 0o644)
    key_path.write_text(Path(config.CLUSTER_CA_KEY_FILE).read_text())
    os.chmod(key_path, 0o600)
    grace_ends = datetime.now(timezone.utc) + timedelta(hours=grace_hours)
    expiry_path.write_text(grace_ends.isoformat())
    logger.info("Workers migrate to the new cluster intermediate CA until %s", grace_ends)


def complete_worker_migration(grace_ends=None) -> None:
    """Drop the old intermediate, ending the worker migration.

    Moves the broker's server cert to the new intermediate and restarts the cluster's own
    MQTT clients, which still hold the old identity and trust. Workers that did not renew
    in time must re-bootstrap with a new token.
    """
    cert_path, key_path, expiry_path = get_old_cluster_ca_paths()
    for path in (cert_path, key_path, expiry_path):
        path.unlink(missing_ok=True)
    write_mqtt_server_identity(config.MY_CHOSEN_CLUSTER_NAME, config.MY_CLUSTER_ADDRESS)
    refresh_cluster_crl()
    logger.warning(
        "Worker migration to the new cluster intermediate CA ended (deadline %s). Workers "
        "that did not renew must re-bootstrap with a new token. Restarting the MQTT clients.",
        grace_ends.isoformat() if grace_ends else "reached early",
    )
    _restart_mqtt_clients_soon()


def _restart_mqtt_clients_soon() -> None:
    """Restart both MQTT clients shortly, leaving time to answer the request first.

    cluster_service_manager is restarted through docker; this worker exits and gunicorn
    starts a new one, which reconnects with the new identity and trust.
    """

    def restart():
        time.sleep(2)
        config.restart_cluster_service_manager()
        os._exit(1)

    threading.Thread(target=restart, daemon=True).start()


def end_expired_worker_migration() -> bool:
    """End the worker migration if its deadline has passed. Returns True if it did."""
    cert_path, _, expiry_path = get_old_cluster_ca_paths()
    if not cert_path.is_file():
        return False
    grace_ends = datetime.fromisoformat(expiry_path.read_text().strip())
    if datetime.now(timezone.utc) < grace_ends:
        return False
    complete_worker_migration(grace_ends)
    return True


@certbp.route("/renew")
class ClusterCertRenewController(MethodView):
    def post(self):
        """Renew this cluster's own mTLS certificate via in-band CSR."""
        success, message = _renew_cluster_certs_in_band()
        if not success:
            abort(502, message=message)
        return {"message": message}


@certbp.route("/rotate")
class ClusterIntermediateRotateController(MethodView):
    def post(self):
        """Replace the cluster intermediate CA, keeping the old one while workers migrate.

        The old intermediate stays trusted for grace_period_hours (default
        INTERMEDIATE_GRACE_PERIOD_HOURS): mosquitto accepts certs from both, and each worker
        is asked to renew through its heartbeat. Workers that have not renewed when the
        migration ends must re-bootstrap with a new token. POST /api/certs/rotate-complete
        ends it early.

        grace_period_hours=0 rotates immediately (e.g. after an intermediate key compromise):
        the old intermediate is never trusted again, every worker must re-bootstrap with a
        new token, and cluster_manager and cluster_service_manager restart right after the
        response.

        Reachable via the internal gateway only.
        """
        content = request.get_json(silent=True) or {}
        try:
            grace_hours = float(
                content.get("grace_period_hours", config.INTERMEDIATE_GRACE_PERIOD_HOURS)
            )
        except (TypeError, ValueError):
            abort(400, message="grace_period_hours must be a number")
        if grace_hours < 0:
            abort(400, message="grace_period_hours must not be negative")

        success, message = _renew_cluster_certs_in_band(
            rotate_intermediate=True, grace_hours=grace_hours
        )
        if not success:
            abort(502, message=message)
        if grace_hours == 0:
            complete_worker_migration()
            message += " Worker migration skipped — only the new intermediate CA is trusted."
        return {"message": message}


@certbp.route("/rotate-complete")
class ClusterIntermediateRotateCompleteController(MethodView):
    def post(self):
        """End the worker migration early, dropping the previous intermediate CA.

        Returns 409 if no migration is in progress. cluster_manager and
        cluster_service_manager restart right after the response.

        Reachable via the internal gateway only.
        """
        if not get_old_cluster_ca_paths()[0].is_file():
            abort(409, message="No worker migration in progress")
        complete_worker_migration()
        return {"message": "Worker migration ended — only the new intermediate CA is trusted."}


def refresh_cluster_crl() -> dict:
    """Drop entries for expired certs, rewrite the cluster CRL from the database and reload MQTT."""
    prune_expired_revoked_certs()
    crl_ok = regenerate_cluster_crl(get_revoked_serials())
    return {"crl_regenerated": crl_ok, "mqtt_reloaded": config.reload_mqtt() if crl_ok else False}


@certbp.route("/revoke")
class ClusterCertRevokeController(MethodView):
    def get(self):
        """List revoked worker certificates. Reachable via the internal gateway only."""
        return {"revoked_certs": list_revoked_certs()}

    @certbp.arguments(schema=revoke_cert_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Revoke a worker certificate by PEM.

        Records the cert's serial in the cluster revoked_certs collection,
        regenerates /certs/cluster_revoked.crl, and reloads MQTT so the
        updated CRL takes effect.
        """
        from cryptography import x509 as _x509

        content = request.get_json(silent=True) or {}
        cert_pem = content.get("cert_pem") or ""
        if not cert_pem.strip():
            abort(400, message="cert_pem is required")

        try:
            cert = _x509.load_pem_x509_certificate(cert_pem.encode("utf-8"))
        except Exception as e:
            abort(400, message=f"Invalid certificate PEM: {e}")

        serial_hex = format(cert.serial_number, "x")
        cert_subject = cert.subject.rfc4514_string()
        store_revoked_cert(
            serial_hex=serial_hex,
            cert_subject=cert_subject,
            not_after=cert.not_valid_after_utc,
            reason=content.get("reason") or "",
            revoked_by="api",
        )

        return {
            "message": "Worker certificate revoked",
            "serial_hex": serial_hex,
            "cert_subject": cert_subject,
            **refresh_cluster_crl(),
        }

    def delete(self):
        """Clear the revocation list, un-revoking every worker certificate."""
        cleared = clear_revoked_certs()
        return {"message": "Revocation list cleared", "cleared": cleared, **refresh_cluster_crl()}


@certbp.route("/revoke/<serial_hex>")
class ClusterCertUnrevokeController(MethodView):
    def delete(self, serial_hex):
        """Un-revoke a single worker certificate by serial number (hex)."""
        if not remove_revoked_cert(serial_hex):
            abort(404, message=f"Serial {serial_hex} is not revoked")
        return {
            "message": "Worker certificate un-revoked",
            "serial_hex": serial_hex,
            **refresh_cluster_crl(),
        }


@certbp.route("/sign")
class ClusterSignCsrController(MethodView):
    @certbp.arguments(schema=sign_csr_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        csr_pem = content.get("csr") or ""
        if not csr_pem.strip():
            abort(400, message="csr is required")

        try:
            fullchain_pem = sign_worker_csr(
                csr_pem=csr_pem,
                valid_days=int(content.get("valid_days") or 365),
            )
        except FileNotFoundError:
            abort(409, message="Cluster intermediate CA material is not initialized")
        except ValueError as e:
            abort(400, message=f"Invalid CSR format: {str(e)}")

        return {"certificate": fullchain_pem}
