import logging
import os

import requests
from ext_requests.certificates import (
    KONG_CA_CERT_UUID,
    ensure_ca_files,
    generate_intermediate_ca,
    generate_key_and_signed_cert,
    get_ca_cert_path,
    get_server_cert_path,
    get_server_key_path,
    regenerate_ca_files,
    regenerate_server_files,
    sign_csr_pem,
)
from ext_requests.registration_tokens_db import TOKEN_TYPE_CLUSTER, consume_registration_token
from flask import request, send_file
from flask_jwt_extended import jwt_required
from flask_restful import Resource
from flask_smorest import abort
from roles.securityUtils import Role, require_role

from blueprints.jwt_wrapper import BlueprintExt

logger = logging.getLogger("system_manager")

certbp = BlueprintExt("Certificates", "certificates", url_prefix="/api/certs")

KONG_ADMIN_URL = os.environ.get("KONG_ADMIN_URL", "http://kong_external:8001")
KONG_CONTAINER_NAME = os.environ.get("KONG_CONTAINER_NAME", "kong_external")
KONG_CA_CERT_NAME = "oakestra-ca-cert"


def _reload_kong_nginx() -> tuple[bool, str]:
    """Send kong reload (nginx SIGHUP) so the updated cert files are picked up without a restart."""
    try:
        import docker

        client = docker.from_env()
        container = client.containers.get(KONG_CONTAINER_NAME)
        result = container.exec_run("kong reload")
        output = result.output.decode("utf-8", errors="replace").strip()
        if result.exit_code == 0:
            logger.info(f"kong reload succeeded: {output}")
            return True, "kong_external reloaded"
        logger.error(f"kong reload exited {result.exit_code}: {output}")
        return False, f"kong reload failed (exit {result.exit_code}): {output}"
    except Exception as e:
        logger.error(f"Could not reload kong: {e}")
        return False, f"Could not reload kong: {str(e)}"


def _update_kong_ca(ca_data: str, old_ca_data: str = None) -> tuple[bool, str]:
    """Update a CA in Kong via Admin API.

    Returns: (success: bool, message: str)
    """
    try:
        response = requests.put(
            f"{KONG_ADMIN_URL}/ca_certificates/{KONG_CA_CERT_UUID}",
            json={"cert": ca_data, "tags": ["oakestra-ca"]},
            timeout=10,
        )
        if response.status_code in (200, 201):
            return True, f"CA certificate updated (id: {KONG_CA_CERT_UUID})"

        if response.status_code == 404:
            response = requests.post(
                f"{KONG_ADMIN_URL}/ca_certificates",
                json={"id": KONG_CA_CERT_UUID, "cert": ca_data, "tags": ["oakestra-ca"]},
                timeout=10,
            )
            if response.status_code in (200, 201):
                return True, f"CA certificate created (id: {KONG_CA_CERT_UUID})"
            return False, f"Failed to create CA: {response.status_code} - {response.text}"

        return False, f"Failed to update CA: {response.status_code} - {response.text}"
    except Exception as e:
        import traceback

        logger.error(f"Error updating CA: {traceback.format_exc()}")
        return False, f"Error updating CA: {str(e)}"


def _update_kong_certificate(old_ca_data: str = None) -> tuple[bool, str]:
    """Update Kong's client-verification CA via the Admin API.

    The gateway's *server* certificate lives in /certs/public/ and is a
    separate, operator-managed (BYO) certificate system — Kong picks it up from
    disk on reload, so only the internal CA entity needs an Admin API update
    here.

    Returns: (success: bool, message: str)
    """
    try:
        ca_path = get_ca_cert_path()
        if not ca_path.exists():
            return False, "CA file not found"

        ca_success, ca_msg = _update_kong_ca(ca_path.read_text(), old_ca_data)
        if not ca_success:
            logger.error(f"Failed to update CA certificate: {ca_msg}")
            return False, f"CA certificate update failed: {ca_msg}"

        logger.info(f"CA certificate: {ca_msg}")
        return True, ca_msg

    except requests.exceptions.ConnectionError:
        logger.error("Cannot connect to Kong Admin API")
        return False, "Cannot connect to Kong Admin API"
    except requests.exceptions.Timeout:
        logger.error("Kong Admin API request timed out")
        return False, "Kong Admin API request timed out"
    except Exception as e:
        logger.error(f"Error updating Kong certificates: {str(e)}")
        return False, f"Error updating Kong certificates: {str(e)}"


# --------- ROUTES ---------

create_ca_schema = {
    "type": "object",
    "properties": {
        "common_name": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
        "server_valid_days": {"type": "integer", "minimum": 1},
        "ca_common_name": {"type": "string"},
        "ca_valid_days": {"type": "integer", "minimum": 1},
    },
}

renew_server_schema = {
    "type": "object",
    "properties": {
        "common_name": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
        "valid_days": {"type": "integer", "minimum": 1},
    },
}

sign_csr_schema = {
    "type": "object",
    "properties": {
        "csr": {"type": "string"},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["csr"],
}


generate_client_schema = {
    "type": "object",
    "properties": {
        "common_name": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["common_name"],
}


generate_cluster_schema = {
    "type": "object",
    "properties": {
        "common_name": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["common_name"],
}


cluster_bootstrap_schema = {
    "type": "object",
    "properties": {
        "token": {"type": "string"},
        "cluster_name": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
    },
    "required": ["token", "cluster_name"],
}


@certbp.route("/reset")
class CertificateAuthorityResetController(Resource):
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self):
        """Rotate the internal CA + server cert."""
        content = request.get_json(silent=True) or {}

        # Generate new CA
        old_ca_data = None
        if get_ca_cert_path().exists():
            old_ca_data = get_ca_cert_path().read_text()

        ca_created = regenerate_ca_files(
            valid_days=int(content.get("valid_days") or 3650),
            common_name=content.get("ca_common_name") or "Oakestra Root CA",
        )

        server_common_name = content.get("common_name") or "localhost"
        server_alt_names = content.get("alt_names") or [server_common_name]
        server_created = regenerate_server_files(
            common_name=server_common_name,
            alt_names=server_alt_names,
            valid_days=int(content.get("server_valid_days") or 365),
        )

        # Update Kong's client-verification CA
        kong_updated, kong_message = _update_kong_certificate(old_ca_data)

        # Reload kong nginx server with new certificates
        reload_ok, reload_message = _reload_kong_nginx()

        return {
            "created": ca_created and server_created,
            "ca_cert": str(get_ca_cert_path()),
            "server_cert": str(get_server_cert_path()),
            "server_key": str(get_server_key_path()),
            "kong_updated": kong_updated,
            "kong_status": kong_message,
            "kong_reloaded": reload_ok,
            "kong_reload_status": reload_message,
            "message": (
                "CA and server certificate material ready, Kong gateway updated and reloaded"
                if kong_updated and reload_ok
                else "CA and server certificate material ready, but some Kong steps failed"
            ),
        }


@certbp.route("/renew-server")
class CertificateServerRenewController(Resource):
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self):
        if not get_ca_cert_path().exists():
            abort(409, {"message": "CA material not initialized — call /reset first"})

        content = request.get_json(silent=True) or {}
        server_common_name = content.get("common_name") or "localhost"
        server_alt_names = content.get("alt_names") or [server_common_name]

        server_created = regenerate_server_files(
            common_name=server_common_name,
            alt_names=server_alt_names,
            valid_days=int(content.get("valid_days") or 365),
        )

        kong_updated, kong_message = _update_kong_certificate()
        reload_ok, reload_message = _reload_kong_nginx()

        return {
            "created": server_created,
            "server_cert": str(get_server_cert_path()),
            "server_key": str(get_server_key_path()),
            "kong_updated": kong_updated,
            "kong_status": kong_message,
            "kong_reloaded": reload_ok,
            "kong_reload_status": reload_message,
            "message": (
                "Server certificate renewed, Kong gateway updated and reloaded"
                if kong_updated and reload_ok
                else "Server certificate renewed, but some Kong steps failed"
            ),
        }


@certbp.route("/ca.crt")
class CertificateAuthorityFileController(Resource):
    def get(self):
        ensure_ca_files()
        if not get_ca_cert_path().exists():
            abort(404, {"message": "CA certificate not found"})
        return send_file(
            get_ca_cert_path(),
            mimetype="application/x-pem-file",
            as_attachment=True,
            download_name="ca.crt",
        )


@certbp.route("/sign")
class CertificateSigningController(Resource):
    @certbp.arguments(schema=sign_csr_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        csr_pem = content.get("csr") or ""
        if not csr_pem.strip():
            abort(400, {"message": "csr is required"})

        try:
            signed_certificate = sign_csr_pem(
                csr_pem=csr_pem,
                valid_days=int(content.get("valid_days") or 365),
            )
        except FileNotFoundError:
            abort(409, {"message": "CA material not initialized"})
        except ValueError as e:
            abort(400, {"message": f"Invalid CSR format: {str(e)}"})

        return {"certificate": signed_certificate}


@certbp.route("/generate-client")
class CertificateClientGenerateController(Resource):
    @certbp.arguments(schema=generate_client_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        common_name = content.get("common_name")
        if not common_name:
            abort(400, {"message": "common_name is required"})

        alt_names = list(content.get("alt_names") or [])
        for name in (common_name, "mqtt", "localhost"):
            if name not in alt_names:
                alt_names.append(name)
        valid_days = int(content.get("valid_days") or 365)

        try:
            private_pem, cert_pem = generate_key_and_signed_cert(
                common_name=common_name, alt_names=alt_names, valid_days=valid_days
            )
        except FileNotFoundError:
            abort(409, {"message": "CA material not initialized"})
        except ValueError as e:
            abort(400, {"message": f"Certificate generation failed: {str(e)}"})

        return {"private_key": private_pem, "certificate": cert_pem}


@certbp.route("/cluster-bootstrap")
class ClusterBootstrapController(Resource):
    @certbp.arguments(
        schema=cluster_bootstrap_schema, location="json", validate=False, unknown=True
    )
    def post(self, *args, **kwargs):
        """Redeem a one-time cluster registration token for cert material.

        Reachable without JWT or client certificate — this is the bootstrap
        path for a cluster that has no trust material yet. The single-use,
        short-lived token (minted via POST /api/tokens/cluster) is the only
        credential. Returns the cluster's client identity, its intermediate
        CA, and the root CA.
        """
        content = request.get_json(silent=True) or {}
        token = content.get("token") or ""
        cluster_name = content.get("cluster_name") or ""
        if not cluster_name.strip():
            abort(400, {"message": "cluster_name is required"})

        # Uniform 401 for missing/unknown/expired/reused tokens — no oracle.
        if consume_registration_token(token, TOKEN_TYPE_CLUSTER) is None:
            abort(401, {"message": "invalid registration token"})

        client_alt_names = list(content.get("alt_names") or [])
        for name in (cluster_name, "mqtt", "localhost"):
            if name not in client_alt_names:
                client_alt_names.append(name)

        try:
            client_key, client_cert = generate_key_and_signed_cert(
                common_name=cluster_name, alt_names=client_alt_names, valid_days=365
            )
            cluster_ca_key, cluster_ca_cert = generate_intermediate_ca(
                common_name=cluster_name,
                alt_names=content.get("alt_names") or None,
                valid_days=1825,
            )
        except FileNotFoundError:
            abort(409, {"message": "Root CA material not initialized"})
        except ValueError as e:
            abort(400, {"message": f"Certificate generation failed: {str(e)}"})

        logger.info(f"Cluster bootstrap: issued certificates for '{cluster_name}'")
        return {
            "client_key": client_key,
            "client_cert": client_cert,
            "cluster_ca_key": cluster_ca_key,
            "cluster_ca_cert": cluster_ca_cert,
            "root_ca": get_ca_cert_path().read_text(),
        }


@certbp.route("/generate-cluster")
class CertificateClusterGenerateController(Resource):
    @certbp.arguments(schema=generate_cluster_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        common_name = content.get("common_name")
        if not common_name:
            abort(400, {"message": "common_name is required"})

        alt_names = content.get("alt_names") or None
        valid_days = int(content.get("valid_days") or 1825)

        try:
            private_pem, cert_pem = generate_intermediate_ca(
                common_name=common_name, alt_names=alt_names, valid_days=valid_days
            )
        except FileNotFoundError:
            abort(409, {"message": "Root CA material not initialized"})
        except ValueError as e:
            abort(400, {"message": f"Intermediate CA generation failed: {str(e)}"})

        root_ca = get_ca_cert_path().read_text()

        return {
            "private_key": private_pem,
            "certificate": cert_pem,
            "root_ca": root_ca,
        }
