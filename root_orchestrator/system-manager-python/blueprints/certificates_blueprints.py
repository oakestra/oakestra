import logging
import os
from datetime import datetime, timedelta, timezone

import requests
from ext_requests.certificates import (
    KONG_CA_CERT_UUID,
    cert_expires_in,
    current_ca_cert_pem,
    ensure_ca_files,
    generate_intermediate_ca,
    generate_key_and_signed_cert,
    get_ca_cert_path,
    get_ca_key_path,
    get_old_ca_cert_path,
    get_old_ca_key_path,
    get_server_cert_path,
    get_server_key_path,
    regenerate_ca_files,
    regenerate_root_crl,
    regenerate_server_files,
    reissue_server_files,
    sign_csr_pem,
)
from ext_requests.registration_tokens_db import TOKEN_TYPE_CLUSTER, consume_registration_token
from ext_requests.revoked_certs_db import (
    add_revoked_cert,
    clear_revoked_certs,
    get_revoked_serials,
    list_revoked_certs,
    prune_expired_revoked_certs,
    remove_revoked_cert,
)
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


# CRLs are signed with a 30-day nextUpdate (see generate_root_crl); re-sign well before expiry.
CRL_REFRESH_INTERVAL_HOURS = float(os.environ.get("CRL_REFRESH_INTERVAL_HOURS") or 24)
# How long the old root CA stays trusted after POST /api/certs/rotate.
ROTATION_GRACE_PERIOD_HOURS = float(os.environ.get("ROTATION_GRACE_PERIOD_HOURS") or 24)
# How often system_manager checks whether the rotation grace period has ended.
ROTATION_CHECK_INTERVAL_SECONDS = 300


# Warn this long before the root CA expires
ROOT_CA_WARN_DAYS = 90


def warn_if_root_ca_expiring() -> None:
    days = cert_expires_in(current_ca_cert_pem()).days
    if days < 0:
        logger.error(
            "The root CA has EXPIRED — no cluster or worker certificate verifies. Rotate it "
            "(POST /api/certs/rotate), then re-register all clusters."
        )
    elif days <= ROOT_CA_WARN_DAYS:
        logger.warning(
            "The root CA expires in %d day(s). Rotate it before then with POST /api/certs/rotate.",
            days,
        )


# Renew the root's server.crt this long before it expires.
SERVER_CERT_RENEW_DAYS = 30


def renew_server_cert_if_expiring() -> None:
    """Re-issue server.crt, the root's client certificate towards clusters, near expiry.

    Skipped during a rotation grace period: clusters that have not renewed yet only
    trust the old CA, and complete_ca_rotation() re-issues it when the period ends.
    """
    if get_old_ca_cert_path().is_file():
        return
    days = cert_expires_in(get_server_cert_path().read_text()).days
    if days > SERVER_CERT_RENEW_DAYS:
        return
    logger.warning("Root server certificate expires in %d day(s); re-issuing it.", days)
    reissue_server_files()


def write_root_crl_from_db() -> bool:
    """Drop entries for expired certs and rewrite /certs/revoked.crl from the database."""
    prune_expired_revoked_certs()
    return regenerate_root_crl(get_revoked_serials())


def refresh_root_crl() -> dict:
    """Rewrite the CRL from the database and reload Kong so the new CRL takes effect."""
    crl_ok = write_root_crl_from_db()
    reload_ok, reload_msg = _reload_kong_nginx() if crl_ok else (False, "CRL was not written")
    return {"crl_regenerated": crl_ok, "kong_reloaded": reload_ok, "kong_reload_status": reload_msg}


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

        # Revocations belong to the replaced CA: clear them and re-sign an empty CRL with the new key.
        revoked_cleared = clear_revoked_certs()
        crl_ok = regenerate_root_crl([])

        # Update Kong's client-verification CA
        kong_updated, kong_message = _update_kong_certificate(old_ca_data)

        # Reload kong nginx server with new certificates
        reload_ok, reload_message = _reload_kong_nginx()

        return {
            "created": ca_created and server_created,
            "ca_cert": str(get_ca_cert_path()),
            "revoked_certs_cleared": revoked_cleared,
            "crl_regenerated": crl_ok,
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


public_cert_schema = {
    "type": "object",
    "properties": {
        "fullchain": {"type": "string"},
        "private_key": {"type": "string"},
    },
    "required": ["fullchain", "private_key"],
}

revoke_cert_schema = {
    "type": "object",
    "properties": {
        "cert_pem": {"type": "string"},
        "reason": {"type": "string"},
    },
    "required": ["cert_pem"],
}

rotate_ca_schema = {
    "type": "object",
    "properties": {
        "ca_common_name": {"type": "string"},
        "ca_valid_days": {"type": "integer", "minimum": 1},
        "grace_period_hours": {"type": "number", "exclusiveMinimum": 0},
    },
}

cluster_renew_schema = {
    "type": "object",
    "properties": {
        "csr": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
        "valid_days": {"type": "integer", "minimum": 1},
        "renew_intermediate": {"type": "boolean"},
    },
    "required": ["csr"],
}


@certbp.route("/revoke")
class CertificateRevokeController(Resource):
    @jwt_required()
    @require_role(Role.ADMIN)
    def get(self):
        """List revoked cluster client certificates."""
        return {"revoked_certs": list_revoked_certs()}

    @certbp.arguments(schema=revoke_cert_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        """Revoke a cluster client certificate by PEM.

        Records the cert's serial number in the revoked_certs collection,
        regenerates /certs/revoked.crl (signed by the root CA), and reloads
        Kong nginx so the updated CRL takes effect immediately.
        """
        from cryptography import x509 as _x509
        from flask_jwt_extended import get_jwt_identity

        content = request.get_json(silent=True) or {}
        cert_pem = content.get("cert_pem") or ""
        if not cert_pem.strip():
            abort(400, {"message": "cert_pem is required"})

        try:
            cert = _x509.load_pem_x509_certificate(cert_pem.encode("utf-8"))
        except Exception as e:
            abort(400, {"message": f"Invalid certificate PEM: {e}"})

        serial_hex = format(cert.serial_number, "x")
        cert_subject = cert.subject.rfc4514_string()
        add_revoked_cert(
            serial_hex=serial_hex,
            cert_subject=cert_subject,
            not_after=cert.not_valid_after_utc,
            reason=content.get("reason") or "",
            revoked_by=get_jwt_identity() or "",
        )

        return {
            "message": "Certificate revoked",
            "serial_hex": serial_hex,
            "cert_subject": cert_subject,
            **refresh_root_crl(),
        }

    @jwt_required()
    @require_role(Role.ADMIN)
    def delete(self):
        """Clear the revocation list, un-revoking every certificate."""
        cleared = clear_revoked_certs()
        return {"message": "Revocation list cleared", "cleared": cleared, **refresh_root_crl()}


@certbp.route("/revoke/<serial_hex>")
class CertificateUnrevokeController(Resource):
    @jwt_required()
    @require_role(Role.ADMIN)
    def delete(self, *args, **kwargs):
        """Un-revoke a single certificate by serial number (hex, as listed by GET /revoke)."""
        serial_hex = kwargs["serial_hex"]
        if not remove_revoked_cert(serial_hex):
            abort(404, {"message": f"Serial {serial_hex} is not revoked"})
        return {"message": "Certificate un-revoked", "serial_hex": serial_hex, **refresh_root_crl()}


@certbp.route("/rotate")
class CertificateAuthorityRotateController(Resource):
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self):
        """Rotate the root CA with a dual-CA grace period.

        Saves the old CA alongside the new one so Kong continues to accept
        client certs signed by either CA. Clusters see the new CA in the response to
        their next /api/information update and renew via the in-band CSR path.
        The grace period ends automatically after grace_period_hours (default
        ROTATION_GRACE_PERIOD_HOURS), or earlier via POST /api/certs/rotate-complete.
        Clusters that have not renewed by then must be re-registered with a new token.
        """
        content = request.get_json(silent=True) or {}
        grace_hours = float(content.get("grace_period_hours") or ROTATION_GRACE_PERIOD_HOURS)
        rotated_at = datetime.now(timezone.utc)

        ca_path = get_ca_cert_path()
        old_ca_pem = ca_path.read_text() if ca_path.is_file() else None
        old_ca_key = get_ca_key_path().read_bytes() if get_ca_key_path().is_file() else None

        # OpenSSL finds a cert's issuer by subject name, and the certs issued here carry
        # no Authority Key Identifier. Both CAs are trusted during the grace period, so
        # they need different names or old-CA certs are checked against the new CA.
        common_name = content.get("ca_common_name") or (
            f"Oakestra Root CA {rotated_at:%Y-%m-%d %H:%M:%S}"
        )
        if old_ca_pem:
            from cryptography import x509 as _x509
            from cryptography.x509.oid import NameOID

            old_ca = _x509.load_pem_x509_certificate(old_ca_pem.encode("utf-8"))
            old_names = old_ca.subject.get_attributes_for_oid(NameOID.COMMON_NAME)
            if old_names and old_names[0].value == common_name:
                abort(400, {"message": "ca_common_name must differ from the current CA's name"})

        regenerate_ca_files(
            valid_days=int(content.get("ca_valid_days") or 3650),
            common_name=common_name,
        )
        new_ca_pem = ca_path.read_text()

        # Write dual-CA bundle: nginx's ssl_client_certificate accepts a
        # concatenated PEM file and will verify client certs against either CA.
        if old_ca_pem and old_ca_key and old_ca_pem.strip() != new_ca_pem.strip():
            get_old_ca_cert_path().write_text(old_ca_pem)
            os.chmod(get_old_ca_cert_path(), 0o644)
            get_old_ca_key_path().write_bytes(old_ca_key)
            os.chmod(get_old_ca_key_path(), 0o600)
            ca_path.write_text(new_ca_pem + "\n" + old_ca_pem)
            grace_ends = rotated_at + timedelta(hours=grace_hours)
            _grace_expiry_path().write_text(grace_ends.isoformat())
            grace_period = True
        else:
            grace_ends = None
            grace_period = False

        # Re-sign the CRL with the new CA key, plus one signed by the old CA key while
        # the grace period lasts, so certs from both CAs pass Kong's CRL check. Old-CA
        # revocations stay in effect until rotate-complete clears them.
        write_root_crl_from_db()

        kong_updated, kong_message = _update_kong_ca(new_ca_pem)
        reload_ok, reload_message = _reload_kong_nginx()

        return {
            "grace_period_active": grace_period,
            "grace_period_ends": grace_ends.isoformat() if grace_ends else None,
            "kong_updated": kong_updated,
            "kong_status": kong_message,
            "kong_reloaded": reload_ok,
            "kong_reload_status": reload_message,
            "message": (
                "CA rotated. Grace period active — old CA still accepted until "
                f"{grace_ends.isoformat()}. Clusters renew on their next information update."
                if grace_period
                else "CA rotated (no prior CA to carry forward)."
            ),
        }


@certbp.route("/cluster-renew")
class ClusterCertRenewController(Resource):
    @certbp.arguments(schema=cluster_renew_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Sign a cluster's CSR with the current CA.

        Protected by the Kong mTLS pre-function guard — the caller must present
        a valid client certificate (old CA still trusted during grace period).
        No JWT required; the mTLS credential is the authentication.
        """
        content = request.get_json(silent=True) or {}
        csr_pem = content.get("csr") or ""
        if not csr_pem.strip():
            abort(400, {"message": "csr is required"})

        alt_names = content.get("alt_names") or []
        valid_days = int(content.get("valid_days") or 365)
        renew_intermediate = content.get("renew_intermediate") is True

        try:
            # Extract CN from the CSR subject to name the new intermediate CA.
            from cryptography import x509 as _x509

            csr_obj = _x509.load_pem_x509_csr(csr_pem.encode("utf-8"))
            cn_attrs = csr_obj.subject.get_attributes_for_oid(_x509.oid.NameOID.COMMON_NAME)
            common_name = cn_attrs[0].value if cn_attrs else "cluster"

            response = {
                "client_cert": sign_csr_pem(csr_pem=csr_pem, valid_days=valid_days),
                "root_ca": get_ca_cert_path().read_text(),
            }
            if renew_intermediate:
                # The cluster trusts the old and new intermediate while its workers migrate,
                # and OpenSSL matches certs and CRLs to their issuer by name, so the names
                # must differ.
                renewed_at = datetime.now(timezone.utc)
                response["cluster_ca_key"], response["cluster_ca_cert"] = generate_intermediate_ca(
                    common_name=f"{common_name} {renewed_at:%Y-%m-%d %H:%M:%S}",
                    alt_names=alt_names or None,
                    valid_days=1825,
                )
        except FileNotFoundError:
            abort(409, {"message": "Root CA material not initialized"})
        except ValueError as e:
            abort(400, {"message": f"Certificate operation failed: {str(e)}"})

        logger.info(
            "Issued renewed cert material for cluster '%s' via in-band CSR (intermediate: %s)",
            common_name,
            "renewed" if renew_intermediate else "kept",
        )
        return response


@certbp.route("/rotate-complete")
class CertificateRotateCompleteController(Resource):
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self):
        """End the dual-CA grace period early by removing the old CA from the bundle.

        Returns 409 if no grace period is active (ca.old.crt not present).
        After this call, only client certs signed by the new CA are accepted.
        """
        if not get_old_ca_cert_path().is_file():
            abort(409, {"message": "No grace period active — ca.old.crt not found"})
        return complete_ca_rotation()


def _grace_expiry_path():
    return get_ca_cert_path().parent / "ca.old.expiry"


def complete_ca_rotation() -> dict:
    """Drop the old CA from the trust bundle, ending the rotation grace period."""
    ca_path = get_ca_cert_path()

    # Strip the old CA from the bundle: keep only the first cert in the file.
    import re as _re

    current_bundle = ca_path.read_text()
    certs = _re.findall(
        r"-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----",
        current_bundle,
        _re.DOTALL,
    )
    if certs:
        ca_path.write_text(certs[0] + "\n")

    get_old_ca_cert_path().unlink()
    get_old_ca_key_path().unlink(missing_ok=True)
    _grace_expiry_path().unlink(missing_ok=True)

    # All old-CA certs are now untrusted at the CA level. Revocation records
    # for them are stale — their serial numbers are scoped to the old CA and
    # carrying them into the new CRL would be wrong. Clear and regenerate.
    clear_revoked_certs()
    regenerate_root_crl([])

    # server.crt is the root's client certificate towards the clusters' gateways. Clusters
    # that renewed during the grace period trust the new CA, so move it over now; the
    # old-CA copy would stop working once clusters drop the old CA from their ca.crt.
    reissue_server_files()

    kong_updated, kong_message = _update_kong_ca(ca_path.read_text())
    reload_ok, reload_message = _reload_kong_nginx()

    return {
        "old_ca_removed": True,
        "revoked_certs_cleared": True,
        "kong_updated": kong_updated,
        "kong_status": kong_message,
        "kong_reloaded": reload_ok,
        "kong_reload_status": reload_message,
        "message": "Grace period ended — only new CA is now trusted.",
    }


def complete_expired_ca_rotation() -> bool:
    """End the rotation grace period if its deadline has passed. Returns True if it did."""
    if not get_old_ca_cert_path().is_file():
        return False
    grace_ends = datetime.fromisoformat(_grace_expiry_path().read_text().strip())
    if datetime.now(timezone.utc) < grace_ends:
        return False

    result = complete_ca_rotation()
    logger.warning(
        "CA rotation grace period ended at %s. "
        "Clusters that did not renew must be re-registered with a new token. %s",
        grace_ends.isoformat(),
        result["kong_reload_status"],
    )
    return True


@certbp.route("/public")
class PublicGatewayCertController(Resource):
    @certbp.arguments(schema=public_cert_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_role(Role.ADMIN)
    def put(self, *args, **kwargs):
        """Replace the BYO public gateway server certificate.

        Writes fullchain.pem and privkey.pem to /certs/public/ then reloads
        Kong so the new cert is served without a container restart.

        Only reachable via the internal gateway — blocked on the external gateway
        so the private key never crosses the public-facing TLS termination point.
        """
        content = request.get_json(silent=True) or {}
        fullchain_pem = (content.get("fullchain") or "").strip()
        private_key_pem = (content.get("private_key") or "").strip()

        if not fullchain_pem or not private_key_pem:
            abort(400, {"message": "fullchain and private_key are required"})

        # Validate PEM and confirm the key matches the certificate.
        try:
            from cryptography import x509 as _x509
            from cryptography.hazmat.primitives import serialization as _ser

            cert = _x509.load_pem_x509_certificate(fullchain_pem.encode())
            key = _ser.load_pem_private_key(private_key_pem.encode(), password=None)

            cert_pub = cert.public_key().public_bytes(
                _ser.Encoding.PEM, _ser.PublicFormat.SubjectPublicKeyInfo
            )
            key_pub = key.public_key().public_bytes(
                _ser.Encoding.PEM, _ser.PublicFormat.SubjectPublicKeyInfo
            )
            if cert_pub != key_pub:
                abort(400, {"message": "private_key does not match the certificate"})
        except ValueError as exc:
            abort(400, {"message": f"Invalid PEM material: {exc}"})

        public_dir = get_ca_cert_path().parent / "public"
        public_dir.mkdir(parents=True, exist_ok=True)
        fullchain_path = public_dir / "fullchain.pem"
        privkey_path = public_dir / "privkey.pem"

        fullchain_path.write_text(fullchain_pem + "\n")
        os.chmod(fullchain_path, 0o644)
        privkey_path.write_text(private_key_pem + "\n")
        os.chmod(privkey_path, 0o600)

        # Kong (nginx) runs as UID/GID 1000 in the kong:3.6 image.
        try:
            os.chown(public_dir, 1000, 1000)
            os.chown(fullchain_path, 1000, 1000)
            os.chown(privkey_path, 1000, 1000)
        except PermissionError:
            logger.warning("Could not chown /certs/public to kong user — reload may fail")

        reload_ok, reload_message = _reload_kong_nginx()
        logger.info("Public gateway certificate updated; kong reload: %s", reload_message)

        return {
            "message": "Public gateway certificate updated",
            "kong_reloaded": reload_ok,
            "kong_reload_status": reload_message,
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
