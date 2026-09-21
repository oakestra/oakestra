"""One-shot certificate bootstrap for the cluster orchestrator.

Runs as the entrypoint of the cluster_cert_bootstrap container, before Kong,
mosquitto, cluster_manager and cluster_service_manager start (they depend on
this container completing successfully).

Responsibilities:

1. If the cluster's mTLS material is missing, or CLUSTER_REGISTRATION_TOKEN is a
   token that has not been redeemed yet, redeem it against the root's public
   (TLS-only, no client cert) /api/certs/cluster-bootstrap endpoint and write the
   five files: ca.crt, cluster.crt, cluster.key, cluster_ca.crt, cluster_ca.key.
   Setting a new token re-registers the cluster: it gets a new intermediate CA, so
   every worker must re-bootstrap afterwards.

2. Verify that a BYO public gateway certificate is present in /certs/public/.
   There is intentionally NO auto-generated fallback — fail fast with an
   actionable error if the operator has not provided fullchain.pem + privkey.pem.

3. (Re-)sign cluster_revoked.crl with the cluster intermediate CA. mosquitto
   refuses to start without it and rejects every client if it is stale.

4. Issue cluster_mqtt.crt/key from the cluster intermediate CA: the MQTT client
   identity of cluster_manager, cluster_service_manager and the mqtt healthcheck.

5. Issue mqtt_server.crt/key from the cluster intermediate CA: mosquitto's server
   certificate, with CLUSTER_ADDRESS, mqtt and localhost as names.

Server verification of the root gateway follows ROOT_GATEWAY_TRUST, interpreted by
config.parse_root_gateway_trust() exactly as in cluster_manager:
  "system"   -> OS trust store (root gateway uses a publicly trusted BYO cert)
  <path>     -> a CA bundle inside the container, e.g. /certs/root-gateway-ca.crt
  ""         -> the internal root CA; bootstrap first fetches it over an unverified
                connection (TOFU) because it does not exist yet
  "insecure" -> no verification of the root gateway certificate
"""

import hashlib
import logging
import os
import sys
from pathlib import Path

import requests

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger("cluster_cert_bootstrap")

CERT_DIR = Path(os.environ.get("CERT_PATH") or "/certs")
ROOT_CA_FILE = CERT_DIR / "ca.crt"
CLUSTER_CERT_FILE = CERT_DIR / "cluster.crt"
CLUSTER_KEY_FILE = CERT_DIR / "cluster.key"
CLUSTER_CA_CERT_FILE = CERT_DIR / "cluster_ca.crt"
CLUSTER_CA_KEY_FILE = CERT_DIR / "cluster_ca.key"
PUBLIC_CERT_FILE = CERT_DIR / "public" / "fullchain.pem"
PUBLIC_KEY_FILE = CERT_DIR / "public" / "privkey.pem"
CLUSTER_CRL_FILE = CERT_DIR / "cluster_revoked.crl"
REDEEMED_TOKEN_FILE = CERT_DIR / "registration_token.sha256"  # Hash of last used token

# nginx in the kong:3.6 image runs as the kong user (UID/GID 1000); the public
# gateway key must be readable by it. mosquitto and the service managers all
# run as root, so only the kong-consumed public material needs this.
KONG_UID = 1000
KONG_GID = 1000

MTLS_FILES = (
    ROOT_CA_FILE,
    CLUSTER_CERT_FILE,
    CLUSTER_KEY_FILE,
    CLUSTER_CA_CERT_FILE,
    CLUSTER_CA_KEY_FILE,
)

REQUEST_TIMEOUT = 30


def _write(path: Path, content: str, mode: int) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)
    os.chmod(path, mode)


def _fetch_root_ca_tofu(base_url: str) -> None:
    # No trust anchor yet: fetch the internal root CA over an unverified connection.
    logger.warning(
        "ROOT_GATEWAY_TRUST is empty — fetching the internal root CA without server "
        "verification (TOFU). Set ROOT_GATEWAY_TRUST=system if the root gateway uses a "
        "publicly trusted certificate."
    )
    import urllib3

    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    response = requests.get(f"{base_url}/api/certs/ca.crt", verify=False, timeout=REQUEST_TIMEOUT)
    response.raise_for_status()
    _write(ROOT_CA_FILE, response.text, 0o644)
    logger.info("Fetched root CA via TOFU -> %s", ROOT_CA_FILE)


def _resolve_verify(base_url: str):
    """Turn ROOT_GATEWAY_TRUST into requests' `verify=` for the token redemption call."""
    if not os.environ.get("ROOT_GATEWAY_TRUST", "").strip():
        _fetch_root_ca_tofu(base_url)

    import config

    try:
        verify = config.root_gateway_verify()
    except ValueError as exc:
        logger.error("%s", exc)
        sys.exit(1)
    if verify is False:
        logger.warning(
            "ROOT_GATEWAY_TRUST=insecure — the root gateway certificate is not verified."
        )
        import urllib3

        urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    return verify


def _token_digest(token: str) -> str:
    return hashlib.sha256(token.encode("utf-8")).hexdigest()


def _is_new_token(token: str) -> bool:
    try:
        return REDEEMED_TOKEN_FILE.read_text().strip() != _token_digest(token)
    except OSError:
        return True


def redeem_cluster_token(token: str) -> None:
    root_url = os.environ.get("SYSTEM_MANAGER_URL") or ""
    root_port = os.environ.get("SYSTEM_MANAGER_PORT") or "443"
    cluster_name = os.environ.get("CLUSTER_NAME") or ""
    cluster_ip = os.environ.get("CLUSTER_ADDRESS") or ""

    if not token:
        logger.error(
            "Cluster certificate material is missing and CLUSTER_REGISTRATION_TOKEN is not set."
        )
        logger.error(
            "Mint a token at the root (POST /api/tokens/cluster, or via the dashboard/CLI) "
            "and re-run with CLUSTER_REGISTRATION_TOKEN=<token>, or provision the files in %s "
            "manually.",
            CERT_DIR,
        )
        sys.exit(1)
    if not root_url or not cluster_name:
        logger.error("SYSTEM_MANAGER_URL and CLUSTER_NAME must be set to bootstrap certificates.")
        sys.exit(1)

    base_url = f"https://{root_url}:{root_port}"
    verify = _resolve_verify(base_url)

    alt_names = [name for name in (cluster_ip,) if name]
    logger.info("Redeeming cluster registration token at %s", base_url)
    response = requests.post(
        f"{base_url}/api/certs/cluster-bootstrap",
        json={"token": token, "cluster_name": cluster_name, "alt_names": alt_names},
        verify=verify,
        timeout=REQUEST_TIMEOUT,
    )
    if response.status_code == 401:
        logger.error(
            "Registration token rejected (invalid, expired, or already used). "
            "Mint a fresh token and retry."
        )
        sys.exit(1)
    if response.status_code != 200:
        logger.error("Cluster bootstrap failed: %s - %s", response.status_code, response.text[:500])
        sys.exit(1)

    payload = response.json()
    returned_root_ca = payload["root_ca"]
    if ROOT_CA_FILE.is_file() and ROOT_CA_FILE.read_text().strip() != returned_root_ca.strip():
        logger.warning(
            "Root CA returned by the bootstrap endpoint differs from the TOFU-fetched copy — "
            "using the bootstrap response."
        )

    _write(ROOT_CA_FILE, returned_root_ca, 0o644)
    _write(CLUSTER_CERT_FILE, payload["client_cert"], 0o644)
    _write(CLUSTER_KEY_FILE, payload["client_key"], 0o600)
    _write(CLUSTER_CA_CERT_FILE, payload["cluster_ca_cert"], 0o644)
    _write(CLUSTER_CA_KEY_FILE, payload["cluster_ca_key"], 0o600)
    _write(REDEEMED_TOKEN_FILE, _token_digest(token), 0o600)
    logger.info("Wrote cluster certificate material to %s", CERT_DIR)


def _make_kong_readable() -> None:
    """Give the kong user (UID/GID 1000) read access to the public material.

    Applied on every run so re-runs and BYO drops both end up kong-readable
    without making the private key world-readable.
    """
    try:
        for path in (PUBLIC_CERT_FILE.parent, PUBLIC_CERT_FILE, PUBLIC_KEY_FILE):
            os.chown(path, KONG_UID, KONG_GID)
        os.chmod(PUBLIC_CERT_FILE, 0o644)
        os.chmod(PUBLIC_KEY_FILE, 0o600)
    except PermissionError:
        logger.warning(
            "Could not chown %s to the kong user — the gateway may fail to read its key.",
            PUBLIC_CERT_FILE.parent,
        )


def ensure_public_gateway_cert() -> None:
    """Require a bring-your-own public gateway certificate.

    The cluster gateway's public-facing identity is a *separate certificate
    system* from the internal mTLS CA — there is no auto-generated fallback.
    The operator must provide fullchain.pem + privkey.pem in /certs/public/.
    """
    if not (PUBLIC_CERT_FILE.is_file() and PUBLIC_KEY_FILE.is_file()):
        logger.error("Public gateway certificate not found.")
        logger.error(
            "The cluster gateway requires a bring-your-own public certificate. "
            "Provide %s and %s (e.g. a Let's Encrypt cert, or your own PKI) and restart.",
            PUBLIC_CERT_FILE,
            PUBLIC_KEY_FILE,
        )
        sys.exit(1)

    logger.info("Public gateway certificate present — leaving content untouched.")
    _make_kong_readable()


WARN_EXPIRY_DAYS = 30


def _check_cert_expiry() -> bool:
    """Return True if the cluster client cert has not yet expired.

    Logs a warning if it expires within WARN_EXPIRY_DAYS; cluster_manager renews it
    automatically once it is running.
    """
    try:
        from datetime import datetime as _dt
        from datetime import timezone as _tz

        from cryptography import x509 as _x509

        pem = CLUSTER_CERT_FILE.read_text()
        cert = _x509.load_pem_x509_certificate(pem.encode("utf-8"))
        remaining = cert.not_valid_after_utc - _dt.now(_tz.utc)
        days = remaining.days

        if remaining.total_seconds() <= 0:
            logger.error(
                "Cluster client certificate expired on %s.", cert.not_valid_after_utc.isoformat()
            )
            return False

        if days <= WARN_EXPIRY_DAYS:
            logger.warning(
                "Cluster client certificate expires in %d day(s) (%s). "
                "Certificate will be renewed.",
                days,
                cert.not_valid_after_utc.isoformat(),
            )
        return True
    except Exception as exc:
        logger.warning("Could not check cluster cert expiry: %s", exc)
        return True


def _use_bootstrap_cert_paths() -> None:
    # ext_requests.cluster_certificates locates the CA files via config, i.e. the
    # environment; point it at this container's cert directory.
    os.environ.setdefault("ROOT_CA_FILE", str(ROOT_CA_FILE))
    os.environ.setdefault("CLUSTER_CA_CERT_FILE", str(CLUSTER_CA_CERT_FILE))
    os.environ.setdefault("CLUSTER_CA_KEY_FILE", str(CLUSTER_CA_KEY_FILE))


def write_cluster_crl() -> None:
    """(Re-)sign the cluster CRL so mosquitto starts with a current list."""
    from cryptography import x509
    from ext_requests.cluster_certificates import load_cluster_ca, regenerate_cluster_crl

    entries = []
    if CLUSTER_CRL_FILE.is_file():
        try:
            intermediate_cert, _, _ = load_cluster_ca()
            crl = x509.load_pem_x509_crl(CLUSTER_CRL_FILE.read_bytes())
            if crl.is_signature_valid(intermediate_cert.public_key()):
                entries = [(entry.serial_number, entry.revocation_date_utc) for entry in crl]
            else:
                logger.info(
                    "Existing cluster CRL was signed by a previous cluster CA — starting empty."
                )
        except Exception as exc:
            logger.warning("Could not read the existing cluster CRL (%s) — starting empty.", exc)

    if not regenerate_cluster_crl(entries):
        logger.error(
            "Could not write %s — mosquitto cannot start without it. Check the cluster CA files.",
            CLUSTER_CRL_FILE,
        )
        sys.exit(1)
    logger.info("Signed cluster CRL with %d revoked certificate(s).", len(entries))


def issue_cluster_mqtt_identity() -> None:
    from ext_requests.cluster_certificates import write_cluster_mqtt_identity

    try:
        cert_path = write_cluster_mqtt_identity(os.environ.get("CLUSTER_NAME") or "")
    except Exception as exc:
        logger.error("Could not issue the cluster MQTT client certificate: %s", exc)
        sys.exit(1)
    logger.info("Issued cluster MQTT client certificate %s", cert_path)


def issue_mqtt_server_identity() -> None:
    from ext_requests.cluster_certificates import write_mqtt_server_identity

    try:
        cert_path = write_mqtt_server_identity(
            os.environ.get("CLUSTER_NAME") or "", os.environ.get("CLUSTER_ADDRESS") or ""
        )
    except Exception as exc:
        logger.error("Could not issue the MQTT broker server certificate: %s", exc)
        sys.exit(1)
    logger.info("Issued MQTT broker server certificate %s", cert_path)


def reload_running_services() -> None:
    """Reload mosquitto and the external gateway"""
    import docker

    client = docker.from_env()
    for name, reload in (
        (os.environ.get("MQTT_CONTAINER_NAME") or "mqtt", lambda c: c.kill("HUP")),
        (
            os.environ.get("KONG_EXTERNAL_CONTAINER_NAME") or "cluster_kong_external",
            lambda c: c.exec_run("kong reload"),
        ),
    ):
        try:
            container = client.containers.get(name)
            if container.status == "running":
                reload(container)
                logger.info("Reloaded the running %s.", name)
        except docker.errors.NotFound:
            pass
        except Exception as exc:
            logger.error("Could not reload %s — restart it manually: %s", name, exc)


def main() -> None:
    # Before anything imports config: it reads the CA locations at import time.
    _use_bootstrap_cert_paths()
    token = os.environ.get("CLUSTER_REGISTRATION_TOKEN") or ""
    reregistered = False
    if not all(path.is_file() for path in MTLS_FILES):
        redeem_cluster_token(token)
    elif token and _is_new_token(token):
        logger.info("NEW CLUSTER_REGISTRATION_TOKEN — re-registering the cluster.")
        redeem_cluster_token(token)
        reregistered = True
    else:
        logger.info("Cluster certificate material already present — skipping token redemption.")
        if not _check_cert_expiry():
            logger.error("Re-register the cluster by setting a new CLUSTER_REGISTRATION_TOKEN.")
            sys.exit(1)
    ensure_public_gateway_cert()
    write_cluster_crl()
    issue_cluster_mqtt_identity()
    issue_mqtt_server_identity()
    if reregistered:
        reload_running_services()
    logger.info("Certificate bootstrap complete.")


if __name__ == "__main__":
    main()
