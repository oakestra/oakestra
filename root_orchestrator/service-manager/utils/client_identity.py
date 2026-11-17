"""Identify the cluster calling through the Oakestra gateway.

Behind the gateway (GATEWAY_ENABLED), the root's external gateway verifies each
cluster's mTLS client certificate and forwards it in the X-Client-Cert header,
URL-encoded. The certificate carries the cluster's id as a SPIFFE ID,
spiffe://oakestra/cluster/<cluster_id>, which its issuer, the root CA, set.

The gateway accepts any chain to the root CA, which also signs every cluster's
intermediate CA. A cluster holds its intermediate's key and could put another
cluster's id into a cert it signs itself, so only a cert the root CA signed
directly counts.
"""

import os
import re
from pathlib import Path
from urllib.parse import unquote

from cryptography import x509
from cryptography.exceptions import InvalidSignature

GATEWAY_ENABLED = os.environ.get("GATEWAY_ENABLED", "").lower() in ("true", "1", "yes")
ROOT_CA_FILE = os.environ.get("ROOT_CA_FILE")

_CLUSTER_ID = re.compile(r"spiffe://oakestra/cluster/([0-9a-f]{24})")


class ClientCertError(Exception):
    """The forwarded client certificate does not identify a cluster."""

    def __init__(self, status: int, message: str):
        super().__init__(message)
        self.status = status


def caller_cluster_id(client_cert_header) -> str:
    """The id of the cluster whose client certificate the gateway forwarded.

    Raises ClientCertError unless it is a cluster certificate signed directly by a
    trusted root CA (during a root CA rotation ROOT_CA_FILE holds both).
    """
    try:
        cert = x509.load_pem_x509_certificate(unquote(client_cert_header or "").encode("utf-8"))
    except ValueError:
        raise ClientCertError(401, "Client certificate required")
    if not _issued_by_root_ca(cert):
        raise ClientCertError(401, "Client certificate was not issued by the root CA")
    match = _CLUSTER_ID.fullmatch(_spiffe_id(cert) or "")
    if not match:
        raise ClientCertError(403, "Client certificate is not a cluster certificate")
    return match.group(1)


def _spiffe_id(cert):
    """The cert's SPIFFE ID, or None unless it carries exactly one URI SAN."""
    try:
        san = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    except x509.ExtensionNotFound:
        return None
    uris = san.get_values_for_type(x509.UniformResourceIdentifier)
    return uris[0] if len(uris) == 1 else None


def _issued_by_root_ca(cert) -> bool:
    if not ROOT_CA_FILE or not Path(ROOT_CA_FILE).is_file():
        return False
    for root_ca in x509.load_pem_x509_certificates(Path(ROOT_CA_FILE).read_bytes()):
        try:
            cert.verify_directly_issued_by(root_ca)
        except (ValueError, TypeError, InvalidSignature):
            continue
        return True
    return False
