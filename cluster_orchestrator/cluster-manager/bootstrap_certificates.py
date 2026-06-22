"""One-shot certificate bootstrap for the cluster orchestrator.

Runs as the entrypoint of the cluster_cert_bootstrap container, before Kong,
mosquitto, cluster_manager and cluster_service_manager start (they depend on
this container completing successfully).

Two responsibilities:

1. If the cluster's mTLS material is missing, redeem the one-time
   CLUSTER_REGISTRATION_TOKEN against the root's public (TLS-only, no client
   cert) /api/certs/cluster-bootstrap endpoint and write the five files:
   ca.crt, cluster.crt, cluster.key, cluster_ca.crt, cluster_ca.key.

2. If no BYO public gateway certificate is present in /certs/public/,
   generate a fallback server cert signed by the cluster's intermediate CA
   so the external gateway can start. BYO files are never touched.

Server verification during bootstrap is controlled by ROOT_GATEWAY_TRUST:
  "system"  -> system trust store (root gateway uses a BYO public cert)
  <path>    -> a custom CA bundle
  ""        -> TOFU: fetch the root CA over an unverified connection first,
               then pin it for the actual token redemption (dev default).
"""

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


def _resolve_verify(base_url: str):
    trust = os.environ.get("ROOT_GATEWAY_TRUST", "")
    if trust == "system":
        return True
    if trust and trust != "insecure":
        return trust

    # TOFU: no trust anchor yet — fetch the root CA over an unverified
    # connection and pin it for the token redemption that follows.
    logger.warning(
        "ROOT_GATEWAY_TRUST not set — fetching root CA without server verification (TOFU). "
        "Set ROOT_GATEWAY_TRUST=system if the root gateway uses a publicly trusted certificate."
    )
    import urllib3

    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    response = requests.get(
        f"{base_url}/api/certs/ca.crt", verify=False, timeout=REQUEST_TIMEOUT
    )
    response.raise_for_status()
    _write(ROOT_CA_FILE, response.text, 0o644)
    logger.info("Fetched root CA via TOFU -> %s", ROOT_CA_FILE)
    return str(ROOT_CA_FILE)


def redeem_cluster_token() -> None:
    token = os.environ.get("CLUSTER_REGISTRATION_TOKEN") or ""
    root_url = os.environ.get("SYSTEM_MANAGER_URL") or ""
    root_port = os.environ.get("SYSTEM_MANAGER_PORT") or "443"
    cluster_name = os.environ.get("CLUSTER_NAME") or ""
    cluster_ip = os.environ.get("CLUSTER_IP") or ""

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
        logger.error(
            "Cluster bootstrap failed: %s - %s", response.status_code, response.text[:500]
        )
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


def main() -> None:
    if all(path.is_file() for path in MTLS_FILES):
        logger.info("Cluster certificate material already present — skipping token redemption.")
    else:
        redeem_cluster_token()
    ensure_public_gateway_cert()
    logger.info("Certificate bootstrap complete.")


if __name__ == "__main__":
    main()
