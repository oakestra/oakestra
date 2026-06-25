import logging
import os
from datetime import datetime, timezone
from pathlib import Path

import config
import requests
from ext_requests.cluster_certificates import (
    generate_worker_cert,
    get_root_ca_pem,
    sign_worker_csr,
)
from ext_requests.token_db import consume_token, store_token_hash
from flask import Response, request
from flask.views import MethodView
from flask_smorest import Blueprint, abort

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
        "common_name": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
    },
    "required": ["token", "common_name"],
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

        No app-level auth: the external gateway route is mTLS-guarded (only
        the root, presenting its client cert, can reach it), and the internal
        gateway is loopback-only.
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

        Reachable without a client certificate — this is the bootstrap path
        for a worker that has no trust material yet. The single-use,
        short-lived token (minted at the root via POST /api/tokens/worker and
        pushed here) is the only credential.
        """
        content = request.get_json(silent=True) or {}
        common_name = content.get("common_name") or ""
        if not common_name.strip():
            abort(400, message="common_name is required")

        # Uniform 401 for missing/unknown/expired/reused tokens — no oracle.
        if consume_token(content.get("token") or "") is None:
            abort(401, message="invalid registration token")

        try:
            private_pem, fullchain_pem = generate_worker_cert(
                common_name=common_name,
                alt_names=content.get("alt_names") or None,
                valid_days=365,
            )
            root_ca_pem = get_root_ca_pem()
        except FileNotFoundError:
            abort(409, message="Cluster intermediate CA material is not initialized")
        except ValueError as e:
            abort(400, message=f"Worker certificate generation failed: {str(e)}")

        logger.info(f"Worker bootstrap: issued certificate for '{common_name}'")
        return {
            "private_key": private_pem,
            "certificate": fullchain_pem,
            "root_ca": root_ca_pem,
        }


cluster_refresh_schema = {
    "type": "object",
    "properties": {
        "token": {"type": "string"},
    },
    "required": ["token"],
}


@certbp.route("/refresh")
class ClusterCertRefreshController(MethodView):
    @certbp.arguments(schema=cluster_refresh_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Re-bootstrap the cluster's mTLS certificate material from the root.

        Accepts a one-time cluster registration token (minted at the root via
        POST /api/tokens/cluster). Rewrites all mTLS cert files in the
        mounted cert directory without rotating the root CA. After this call,
        restart mqtt, cluster_manager, and cluster_service_manager for the new
        certificates to take effect.

        Reachable via the internal gateway only.
        """
        content = request.get_json(silent=True) or {}
        token = content.get("token") or ""
        if not token.strip():
            abort(400, message="token is required")

        root_url = os.environ.get("SYSTEM_MANAGER_URL") or ""
        root_port = os.environ.get("SYSTEM_MANAGER_PORT") or "443"
        cluster_name = config.MY_CHOSEN_CLUSTER_NAME or ""
        cluster_ip = config.MY_CLUSTER_IP or ""

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
        mqtt_reloaded = config.reload_mqtt()
        return {
            "message": (
                "Certificates refreshed and MQTT broker reloaded."
                if mqtt_reloaded
                else "Certificates refreshed. MQTT broker reload failed — restart mqtt manually."
            ),
            "mqtt_reloaded": mqtt_reloaded,
            "cert_dir": str(cert_dir),
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
