import logging
import os
from datetime import datetime, timedelta, timezone

from cryptography import x509
from ext_requests.certificates_db import (
    TOKEN_TYPE_CLUSTER,
    add_revoked_cert,
    clear_revoked_certs,
    consume_registration_token,
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
from utils.certificates import (
    ClientCertError,
    cert_expires_in,
    client_cert_cluster_id,
    client_spiffe_id,
    cluster_spiffe_id,
    current_ca_cert_pem,
    generate_ca_material,
    generate_intermediate_ca,
    generate_key_and_signed_cert,
    get_ca_cert_path,
    get_ca_key_path,
    get_old_ca_cert_path,
    get_old_ca_key_path,
    get_server_cert_path,
    get_server_key_path,
    is_ca_cert,
    issued_by_root_ca,
    regenerate_root_crl,
    regenerate_server_files,
    reissue_server_files,
    sign_csr_pem,
    write_file_atomic,
)

from blueprints.jwt_wrapper import BlueprintExt

logger = logging.getLogger("system_manager")

certbp = BlueprintExt("Certificates", "certificates", url_prefix="/api/certs")

# Certificate files are only written here: kong_external reloads itself when ca.crt or
# revoked.crl change on disk (config/cert-watch.sh), within about 15 seconds.


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

    Also re-issued when the current CA did not sign it, which happens when a CA rotation
    is interrupted before complete_ca_rotation() re-issued it.

    Skipped during a rotation grace period: clusters that have not renewed yet only
    trust the old CA, and complete_ca_rotation() re-issues it when the period ends.
    """
    if get_old_ca_cert_path().is_file():
        return
    server_cert_pem = get_server_cert_path().read_text()
    if not issued_by_root_ca(x509.load_pem_x509_certificate(server_cert_pem.encode("utf-8"))):
        logger.warning("Root server certificate is not signed by the root CA; re-issuing it.")
        reissue_server_files()
        return
    days = cert_expires_in(server_cert_pem).days
    if days > SERVER_CERT_RENEW_DAYS:
        return
    logger.warning("Root server certificate expires in %d day(s); re-issuing it.", days)
    reissue_server_files()


def write_root_crl_from_db() -> bool:
    """Drop entries for expired certs and rewrite /certs/revoked.crl from the database."""
    prune_expired_revoked_certs()
    return regenerate_root_crl(get_revoked_serials())


def refresh_root_crl() -> dict:
    """Rewrite the CRL from the database; Kong reloads itself when the file changes."""
    return {"crl_regenerated": write_root_crl_from_db()}


# --------- ROUTES ---------

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
        "common_name": {"type": "string"},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["csr", "common_name"],
}


generate_client_schema = {
    "type": "object",
    "properties": {
        "common_name": {"type": "string"},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["common_name"],
}


cluster_bootstrap_schema = {
    "type": "object",
    "properties": {
        "token": {"type": "string"},
        "cluster_name": {"type": "string"},
    },
    "required": ["token"],
}


@certbp.route("/renew")
class CertificateServerRenewController(Resource):
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self):
        """Re-issue the root's server.crt, its client certificate towards the clusters.

        Also renewed automatically 30 days before expiry and when a CA rotation ends.
        """
        content = request.get_json(silent=True) or {}
        server_common_name = content.get("common_name") or "localhost"
        server_alt_names = content.get("alt_names") or [server_common_name]

        server_created = regenerate_server_files(
            common_name=server_common_name,
            alt_names=server_alt_names,
            valid_days=int(content.get("valid_days") or 365),
        )

        return {
            "created": server_created,
            "server_cert": str(get_server_cert_path()),
            "server_key": str(get_server_key_path()),
            "message": "Server certificate renewed",
        }


@certbp.route("/ca.crt")
class CertificateAuthorityFileController(Resource):
    def get(self):
        if not get_ca_cert_path().exists():
            abort(404, message="CA certificate not found")
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
        """Sign a CSR as an ad-hoc client certificate.

        Only the CSR's public key is used. The certificate is a clientAuth leaf for the
        identity spiffe://oakestra/client/<common_name>, which no machine-to-machine route
        accepts.
        """
        content = request.get_json(silent=True) or {}
        csr_pem = content.get("csr") or ""
        common_name = content.get("common_name") or ""
        if not csr_pem.strip():
            abort(400, message="csr is required")

        try:
            signed_certificate = sign_csr_pem(
                csr_pem=csr_pem,
                common_name=common_name,
                alt_names=[client_spiffe_id(common_name)],
                valid_days=int(content.get("valid_days") or 365),
            )
        except FileNotFoundError:
            abort(409, message="CA material not initialized")
        except ValueError as e:
            abort(400, message=f"Invalid CSR format: {str(e)}")

        return {"certificate": signed_certificate}


@certbp.route("/generate-client")
class CertificateClientGenerateController(Resource):
    @certbp.arguments(schema=generate_client_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        """Generate a key and an ad-hoc client certificate.

        The certificate is a clientAuth leaf for the identity
        spiffe://oakestra/client/<common_name>, which no machine-to-machine route accepts.
        """
        content = request.get_json(silent=True) or {}
        common_name = content.get("common_name") or ""
        valid_days = int(content.get("valid_days") or 365)

        try:
            private_pem, cert_pem = generate_key_and_signed_cert(
                common_name=common_name,
                alt_names=[client_spiffe_id(common_name)],
                valid_days=valid_days,
            )
        except FileNotFoundError:
            abort(409, message="CA material not initialized")
        except ValueError as e:
            abort(400, message=f"Certificate generation failed: {str(e)}")

        return {"private_key": private_pem, "certificate": cert_pem}


def _issue_cluster_material(cluster_id: str) -> tuple[str, str, str, str]:
    """A cluster's client cert and intermediate CA, as (key, cert, ca_key, ca_cert) PEMs.

    The client cert is clientAuth only: the cluster never serves TLS with it.
    """
    client_key, client_cert = generate_key_and_signed_cert(
        common_name=cluster_id, alt_names=[cluster_spiffe_id(cluster_id)], valid_days=365
    )
    cluster_ca_key, cluster_ca_cert = generate_intermediate_ca(cluster_id, valid_days=1825)
    return client_key, client_cert, cluster_ca_key, cluster_ca_cert


@certbp.route("/cluster-bootstrap")
class ClusterBootstrapController(Resource):
    @certbp.arguments(
        schema=cluster_bootstrap_schema, location="json", validate=False, unknown=True
    )
    def post(self, *args, **kwargs):
        """Redeem a one-time cluster registration token for cert material"""
        """Redeem a one-time cluster registration token for cert material.

        The certificates are issued for the cluster id bound to the token.
        """
        content = request.get_json(silent=True) or {}
        token = content.get("token") or ""
        cluster_name = content.get("cluster_name") or ""

        # Uniform 401 for missing/unknown/expired/reused tokens — no oracle.
        token_doc = consume_registration_token(token, TOKEN_TYPE_CLUSTER)
        if token_doc is None or not token_doc.get("cluster_id"):
            abort(401, message="invalid registration token")
        cluster_id = token_doc["cluster_id"]

        try:
            client_key, client_cert, cluster_ca_key, cluster_ca_cert = _issue_cluster_material(
                cluster_id
            )
        except FileNotFoundError:
            abort(409, message="Root CA material not initialized")
        except ValueError as e:
            abort(400, message=f"Certificate generation failed: {str(e)}")

        logger.info(
            f"Cluster bootstrap: issued certificates for cluster {cluster_id} '{cluster_name}'"
        )
        return {
            "cluster_id": cluster_id,
            "client_key": client_key,
            "client_cert": client_cert,
            "cluster_ca_key": cluster_ca_key,
            "cluster_ca_cert": cluster_ca_cert,
            "root_ca": get_ca_cert_path().read_text(),
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
        "grace_period_hours": {"type": "number", "minimum": 0},
    },
}

cluster_renew_schema = {
    "type": "object",
    "properties": {
        "csr": {"type": "string"},
        "valid_days": {"type": "integer", "minimum": 1},
        "rotate_intermediate": {"type": "boolean"},
    },
    "required": ["csr"],
}

# Upper bound for the validity a cluster may ask for when renewing its client cert.
CLUSTER_CERT_MAX_VALID_DAYS = 365


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

        Records the cert's serial number in the revoked_certs collection and
        regenerates /certs/revoked.crl (signed by the root CA); Kong reloads itself
        when the file changes. Only unexpired leaf certs the root CA issued can be
        revoked here: the CRL has no effect on any other certificate.
        """
        from cryptography import x509 as _x509
        from flask_jwt_extended import get_jwt_identity

        content = request.get_json(silent=True) or {}
        cert_pem = content.get("cert_pem") or ""
        if not cert_pem.strip():
            abort(400, message="cert_pem is required")

        try:
            cert = _x509.load_pem_x509_certificate(cert_pem.encode("utf-8"))
        except Exception as e:
            abort(400, message=f"Invalid certificate PEM: {e}")
        check_revocable(cert)

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


def check_revocable(cert) -> None:
    """Abort with 400 unless the root CRL can revoke cert."""
    if is_ca_cert(cert):
        abort(400, message="A CA certificate cannot be revoked; rotate the CA instead")
    if not issued_by_root_ca(cert):
        abort(
            400,
            message="Certificate was not issued by the root CA, so the root CRL cannot revoke it",
        )
    if cert.not_valid_after_utc <= datetime.now(timezone.utc):
        abort(400, message="Certificate has already expired; the gateway rejects it anyway")


@certbp.route("/revoke/<serial_hex>")
class CertificateUnrevokeController(Resource):
    @jwt_required()
    @require_role(Role.ADMIN)
    def delete(self, *args, **kwargs):
        """Un-revoke a single certificate by serial number (hex, as listed by GET /revoke)."""
        serial_hex = kwargs["serial_hex"]
        if not remove_revoked_cert(serial_hex):
            abort(404, message=f"Serial {serial_hex} is not revoked")
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

        grace_period_hours=0 rotates immediately (e.g. after a CA key compromise): the old
        CA is never trusted again and every cluster must be re-registered with a new token.

        Returns 409 if a grace period is already active, unless grace_period_hours=0.
        """
        content = request.get_json(silent=True) or {}
        try:
            grace_hours = float(content.get("grace_period_hours", ROTATION_GRACE_PERIOD_HOURS))
        except (TypeError, ValueError):
            abort(400, message="grace_period_hours must be a number")
        if grace_hours < 0:
            abort(400, message="grace_period_hours must not be negative")
        roll_back_interrupted_ca_rotation()
        # ca.crt holds both CAs during a grace period, and only one of them can be kept
        # as ca.old.*: the other would lose its CRL and with it its grace period.
        if grace_hours > 0 and get_old_ca_cert_path().is_file():
            abort(
                409,
                message=(
                    "A CA rotation grace period is already active. End it first "
                    "(POST /api/certs/rotate-complete) or rotate with grace_period_hours=0."
                ),
            )
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
                abort(400, message="ca_common_name must differ from the current CA's name")

        new_ca_key, new_ca_cert = generate_ca_material(
            valid_days=int(content.get("ca_valid_days") or 3650),
            common_name=common_name,
        )
        new_ca_pem = new_ca_cert.decode("utf-8")

        if get_old_ca_cert_path().is_file():
            # Only reachable with grace_hours=0, which ends the running grace period anyway.
            # Ending it first leaves only the current CA in ca.crt to back up below.
            drop_old_ca()
            old_ca_pem = ca_path.read_text()

        grace_period = bool(old_ca_pem and old_ca_key) and grace_hours > 0
        grace_ends = rotated_at + timedelta(hours=grace_hours) if grace_period else None
        if old_ca_pem and old_ca_key:
            # Back up the old CA before replacing it, so a failure part-way never loses it.
            # ca.old.crt goes last: its presence is what marks a grace period as active.
            # Until ca.crt is rewritten, ca.old.crt equals it and a rollback restores the old CA.
            try:
                expiry = (grace_ends or rotated_at).isoformat()
                write_file_atomic(_grace_expiry_path(), expiry, 0o644)
                write_file_atomic(get_old_ca_key_path(), old_ca_key, 0o600)
                write_file_atomic(get_old_ca_cert_path(), old_ca_pem, 0o644)
                write_file_atomic(get_ca_key_path(), new_ca_key, 0o600)
                # Dual-CA bundle: Kong verifies client certs against either CA.
                bundle = new_ca_pem + "\n" + old_ca_pem if grace_period else new_ca_pem
                write_file_atomic(ca_path, bundle, 0o644)
            except Exception:
                roll_back_interrupted_ca_rotation()
                raise
        else:
            write_file_atomic(get_ca_key_path(), new_ca_key, 0o600)
            write_file_atomic(ca_path, new_ca_pem, 0o644)

        if grace_hours == 0:
            # No grace period: complete the rotation straight away, which also ends one
            # still in progress, clears revocations and re-issues server.crt.
            result = complete_ca_rotation()
            if result["crl_regenerated"]:
                result["message"] = (
                    "CA rotated without a grace period — only the new CA is trusted. "
                    "Every cluster must be re-registered with a new token."
                )
            return {**result, "grace_period_active": False, "grace_period_ends": None}

        # Re-sign the CRL with the new CA key, plus one signed by the old CA key while
        # the grace period lasts, so certs from both CAs pass Kong's CRL check. Old-CA
        # revocations stay in effect until rotate-complete clears them.
        crl_ok = write_root_crl_from_db()
        if not crl_ok:
            message = crl_failed_after_rotation()
        elif grace_period:
            message = (
                "CA rotated. Grace period active — old CA still accepted until "
                f"{grace_ends.isoformat()}. Clusters renew on their next information update."
            )
        else:
            message = "CA rotated (no prior CA to carry forward)."

        return {
            "grace_period_active": grace_period,
            "grace_period_ends": grace_ends.isoformat() if grace_ends else None,
            "crl_regenerated": crl_ok,
            "message": message,
        }


def crl_failed_after_rotation() -> str:
    """Log and return the message for a CA rotation whose CRL could not be re-signed."""
    message = (
        "The CA was rotated, but the CRL could not be written, so the gateway rejects every "
        "certificate the new CA signed. Fix the cause logged above, then restart "
        "system_manager to rewrite the CRL."
    )
    logger.error(message)
    return message


@certbp.route("/cluster-renew")
class ClusterCertRenewController(Resource):
    @certbp.arguments(schema=cluster_renew_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Renew a cluster's client certificate, authenticated by its current certificate.

        The external gateway forwards the verified client certificate (X-Client-Cert); during
        a rotation grace period it may be signed by the old CA. The new certificate is for
        the presented certificate's cluster id: only the CSR's public key is used. With
        rotate_intermediate, a new intermediate CA for the same cluster is issued as well.
        """
        cluster_id = verified_cluster_id(request.headers.get("X-Client-Cert"))

        content = request.get_json(silent=True) or {}
        csr_pem = content.get("csr") or ""
        if not csr_pem.strip():
            abort(400, message="csr is required")
        try:
            valid_days = min(int(content.get("valid_days") or 365), CLUSTER_CERT_MAX_VALID_DAYS)
        except (TypeError, ValueError):
            abort(400, message="valid_days must be an integer")
        rotate_intermediate = content.get("rotate_intermediate") is True

        try:
            response = {
                "client_cert": sign_csr_pem(
                    csr_pem=csr_pem,
                    common_name=cluster_id,
                    alt_names=[cluster_spiffe_id(cluster_id)],
                    valid_days=valid_days,
                ),
                "root_ca": get_ca_cert_path().read_text(),
            }
            if rotate_intermediate:
                # The cluster trusts the old and new intermediate while its workers migrate;
                # generate_intermediate_ca gives the new one a distinct name.
                response["cluster_ca_key"], response["cluster_ca_cert"] = generate_intermediate_ca(
                    cluster_id, valid_days=1825
                )
        except FileNotFoundError:
            abort(409, message="Root CA material not initialized")
        except ValueError as e:
            abort(400, message=f"Certificate operation failed: {str(e)}")

        logger.info(
            "Issued renewed cert material for cluster %s via in-band CSR (intermediate: %s)",
            cluster_id,
            "rotated" if rotate_intermediate else "kept",
        )
        return response


def verified_cluster_id(client_cert_header) -> str:
    """The cluster id of the client cert the external gateway forwarded; aborts if there is none."""
    try:
        return client_cert_cluster_id(client_cert_header)
    except ClientCertError as e:
        abort(e.status, message=str(e))
    except FileNotFoundError:
        abort(409, message="Root CA material not initialized")


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
            abort(409, message="No grace period active — ca.old.crt not found")
        return complete_ca_rotation()


def _grace_expiry_path():
    return get_ca_cert_path().parent / "ca.old.expiry"


def drop_old_ca() -> None:
    """Remove the old CA from the trust bundle, then delete its files.

    ca.old.crt is deleted before the other ca.old.* files: without it nothing reads them.
    """
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
        write_file_atomic(ca_path, certs[0] + "\n", 0o644)

    get_old_ca_cert_path().unlink(missing_ok=True)
    get_old_ca_key_path().unlink(missing_ok=True)
    _grace_expiry_path().unlink(missing_ok=True)


def complete_ca_rotation() -> dict:
    """Drop the old CA from the trust bundle, ending the rotation grace period."""
    drop_old_ca()

    # All old-CA CRL
    clear_revoked_certs()
    crl_ok = regenerate_root_crl([])

    # update server certificate files for new CA
    reissue_server_files()

    return {
        "old_ca_removed": True,
        "revoked_certs_cleared": True,
        "crl_regenerated": crl_ok,
        "message": (
            "Grace period ended — only new CA is now trusted."
            if crl_ok
            else crl_failed_after_rotation()
        ),
    }


def roll_back_interrupted_ca_rotation() -> bool:
    """Restore the old CA if a rotation stopped before rewriting ca.crt. Returns True if it did.

    A rotation backs up the old CA as ca.old.* and writes ca.crt last, so until then
    ca.old.crt equals ca.crt while ca.key may already hold the new key.
    """
    old_cert_path = get_old_ca_cert_path()
    if not old_cert_path.is_file() or old_cert_path.read_bytes() != get_ca_cert_path().read_bytes():
        return False

    write_file_atomic(get_ca_key_path(), get_old_ca_key_path().read_bytes(), 0o600)
    old_cert_path.unlink()
    get_old_ca_key_path().unlink(missing_ok=True)
    _grace_expiry_path().unlink(missing_ok=True)
    logger.warning("A CA rotation was interrupted; the previous CA was restored. Rotate again.")
    return True


def complete_expired_ca_rotation() -> bool:
    """End the rotation grace period if its deadline has passed. Returns True if it did."""
    if not get_old_ca_cert_path().is_file():
        return False
    grace_ends = datetime.fromisoformat(_grace_expiry_path().read_text().strip())
    if datetime.now(timezone.utc) < grace_ends:
        return False

    complete_ca_rotation()
    logger.warning(
        "CA rotation grace period ended at %s. "
        "Clusters that did not renew must be re-registered with a new token.",
        grace_ends.isoformat(),
    )
    return True
