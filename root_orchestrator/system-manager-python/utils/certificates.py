import ipaddress
import os
import re
from datetime import datetime, timedelta, timezone
from pathlib import Path
from urllib.parse import unquote

from cryptography import x509
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

DEFAULT_CERT_PATH = "/certs"
DEFAULT_CA_COMMON_NAME = "Oakestra Root CA"
DEFAULT_CA_VALID_DAYS = 3650
DEFAULT_KEY_SIZE = 3072

# Every CA subject is chosen here, never by a requester, and carries one of these markers.
# The gateway tells "issued directly by the root CA" from "issued by a cluster CA" by them.
ROOT_CA_MARKER = "oakestra-root-ca"
CLUSTER_CA_MARKER = "oakestra-cluster-ca"

# Identities are SPIFFE IDs in a URI SAN, always set by the issuer:
#   spiffe://oakestra/root                               the root (server.crt)
#   spiffe://oakestra/cluster/<cluster_id>               a cluster's client cert
#   spiffe://oakestra/cluster/<cluster_id>/worker/<id>   a worker (issued by the cluster)
#   spiffe://oakestra/client/<name>                      an admin-issued client cert
SPIFFE_PREFIX = "spiffe://oakestra"
ROOT_SPIFFE_ID = f"{SPIFFE_PREFIX}/root"
_OBJECT_ID = re.compile(r"[0-9a-f]{24}")
_CLIENT_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}")


def cluster_spiffe_id(cluster_id: str) -> str:
    if not _OBJECT_ID.fullmatch(cluster_id or ""):
        raise ValueError(f"Invalid cluster id: {cluster_id!r}")
    return f"{SPIFFE_PREFIX}/cluster/{cluster_id}"


def client_spiffe_id(name: str) -> str:
    if not _CLIENT_NAME.fullmatch(name or ""):
        raise ValueError("Client name must be 1-64 letters, digits, '.', '_' or '-'")
    return f"{SPIFFE_PREFIX}/client/{name}"


def spiffe_id(cert: x509.Certificate):
    """The cert's SPIFFE ID, or None unless it carries exactly one URI SAN."""
    try:
        san = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    except x509.ExtensionNotFound:
        return None
    uris = san.get_values_for_type(x509.UniformResourceIdentifier)
    return uris[0] if len(uris) == 1 else None


def cert_cluster_id(cert: x509.Certificate):
    """The cluster id of a cluster client cert, or None for any other identity."""
    identity = spiffe_id(cert) or ""
    prefix = f"{SPIFFE_PREFIX}/cluster/"
    if not identity.startswith(prefix):
        return None
    cluster_id = identity[len(prefix) :]
    return cluster_id if _OBJECT_ID.fullmatch(cluster_id) else None


def get_cert_path() -> Path:
    cert_path = Path(os.environ.get("CERT_PATH") or DEFAULT_CERT_PATH)
    cert_path.mkdir(parents=True, exist_ok=True)
    return cert_path


def get_ca_key_path() -> Path:
    return get_cert_path() / "ca.key"


def get_ca_cert_path() -> Path:
    return get_cert_path() / "ca.crt"


def get_old_ca_cert_path() -> Path:
    """The previous CA, kept only during a rotation grace period."""
    return get_cert_path() / "ca.old.crt"


def get_old_ca_key_path() -> Path:
    """The previous CA's key, kept only during a rotation grace period to sign its CRL."""
    return get_cert_path() / "ca.old.key"


def get_server_key_path() -> Path:
    return get_cert_path() / "server.key"


def get_server_cert_path() -> Path:
    return get_cert_path() / "server.crt"


def write_file_atomic(path: Path, data, mode: int) -> None:
    """Replace path with data in one step, so a reader never sees a half-written file."""
    if isinstance(data, str):
        data = data.encode("utf-8")
    tmp_path = path.with_name(f".{path.name}.tmp")
    tmp_path.write_bytes(data)
    os.chmod(tmp_path, mode)
    os.replace(tmp_path, path)


def ca_files_exist() -> bool:
    return get_ca_key_path().is_file() and get_ca_cert_path().is_file()


def regenerate_ca_files(
    valid_days: int,
    common_name: str = DEFAULT_CA_COMMON_NAME,
) -> bool:
    """Force-generate a new CA key+cert, overwriting any existing material."""
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)
    now = datetime.now(timezone.utc)
    subject = x509.Name(
        [
            x509.NameAttribute(NameOID.ORGANIZATIONAL_UNIT_NAME, ROOT_CA_MARKER),
            x509.NameAttribute(NameOID.COMMON_NAME, common_name),
        ]
    )

    certificate = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(subject)
        .public_key(private_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(days=valid_days))
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .add_extension(
            x509.SubjectKeyIdentifier.from_public_key(private_key.public_key()),
            critical=False,
        )
        .add_extension(
            x509.AuthorityKeyIdentifier.from_issuer_public_key(private_key.public_key()),
            critical=False,
        )
        .sign(private_key, hashes.SHA256())
    )

    get_cert_path()
    write_file_atomic(
        get_ca_key_path(),
        private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.TraditionalOpenSSL,
            encryption_algorithm=serialization.NoEncryption(),
        ),
        0o600,
    )
    write_file_atomic(
        get_ca_cert_path(), certificate.public_bytes(serialization.Encoding.PEM), 0o644
    )
    return True


def ensure_ca_files(
    common_name: str = DEFAULT_CA_COMMON_NAME,
    valid_days: int = DEFAULT_CA_VALID_DAYS,
) -> bool:
    """Generate CA key+cert only if they are missing. Returns True if generated, False if already present."""
    if ca_files_exist():
        return False
    return regenerate_ca_files(common_name=common_name, valid_days=valid_days)


def current_ca_cert_pem() -> str:
    """PEM of the CA that signs new certs; during a rotation grace period ca.crt also
    holds the old CA after it, so only the first cert is returned."""
    ca_cert = x509.load_pem_x509_certificate(get_ca_cert_path().read_bytes())
    return ca_cert.public_bytes(serialization.Encoding.PEM).decode("utf-8")


def load_ca_material():
    if not ca_files_exist():
        raise FileNotFoundError("CA files are missing")

    ca_cert = x509.load_pem_x509_certificate(get_ca_cert_path().read_bytes())
    ca_key = serialization.load_pem_private_key(get_ca_key_path().read_bytes(), password=None)
    return ca_cert, ca_key


def _normalize_pem(pem_data: str, expected_type: str) -> str:
    """Normalize and validate PEM format. Raises ValueError if invalid."""
    pem_str = pem_data.strip() if isinstance(pem_data, str) else pem_data.decode("utf-8").strip()

    begin_marker = f"-----BEGIN {expected_type}-----"
    end_marker = f"-----END {expected_type}-----"

    if begin_marker not in pem_str or end_marker not in pem_str:
        raise ValueError(
            f"Invalid PEM format. Expected {expected_type}. "
            f"PEM must contain '{begin_marker}' and '{end_marker}' markers."
        )

    return pem_str


def load_csr(csr_pem: str) -> x509.CertificateSigningRequest:
    """Parse a CSR and check it was signed by the key it carries."""
    try:
        normalized_csr = _normalize_pem(csr_pem, "CERTIFICATE REQUEST")
        csr = x509.load_pem_x509_csr(normalized_csr.encode("utf-8"))
    except ValueError as ve:
        raise ValueError(f"CSR parsing failed: {str(ve)}")
    except Exception as e:
        raise ValueError(f"Failed to load CSR: {str(e)}")
    if not csr.is_signature_valid:
        raise ValueError("CSR signature is invalid")
    return csr


def _build_san(alt_names: list[str]) -> x509.SubjectAlternativeName:
    san_list = []
    for name in alt_names:
        if name.startswith(SPIFFE_PREFIX):
            san_list.append(x509.UniformResourceIdentifier(name))
            continue
        try:
            san_list.append(x509.IPAddress(ipaddress.ip_address(name)))
        except ValueError:
            san_list.append(x509.DNSName(name))
    return x509.SubjectAlternativeName(san_list)


def _issue_leaf(
    public_key,
    common_name: str,
    alt_names: list[str],
    valid_days: int,
    extended_key_usages: list,
) -> str:
    """Sign a leaf certificate for public_key with the root CA. Returns its PEM.

    Every name and extension is set here. Nothing comes from the requester, so a CSR
    cannot ask for a CA certificate, other key usages or another identity.
    """
    ca_cert, ca_key = load_ca_material()
    now = datetime.now(timezone.utc)

    builder = (
        x509.CertificateBuilder()
        .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)]))
        .issuer_name(ca_cert.subject)
        .public_key(public_key)
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        # Nothing verifies past the root CA's expiry, so never claim longer.
        .not_valid_after(min(now + timedelta(days=valid_days), ca_cert.not_valid_after_utc))
        .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
        .add_extension(
            x509.KeyUsage(
                digital_signature=True,
                content_commitment=False,
                key_encipherment=True,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=False,
                crl_sign=False,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .add_extension(x509.ExtendedKeyUsage(extended_key_usages), critical=False)
        .add_extension(x509.SubjectKeyIdentifier.from_public_key(public_key), critical=False)
        .add_extension(
            x509.AuthorityKeyIdentifier.from_issuer_public_key(ca_cert.public_key()),
            critical=False,
        )
    )
    if alt_names:
        builder = builder.add_extension(_build_san(alt_names), critical=False)

    certificate = builder.sign(private_key=ca_key, algorithm=hashes.SHA256())
    return certificate.public_bytes(serialization.Encoding.PEM).decode("utf-8")


def sign_csr_pem(
    csr_pem: str,
    common_name: str,
    alt_names: list[str],
    valid_days: int,
    extended_key_usages: list = None,
) -> str:
    """Sign the CSR's public key with the given identity; only the key is taken from the CSR.

    extended_key_usages defaults to clientAuth only.
    """
    csr = load_csr(csr_pem)
    return _issue_leaf(
        csr.public_key(),
        common_name=common_name,
        alt_names=alt_names,
        valid_days=valid_days,
        extended_key_usages=extended_key_usages or [ExtendedKeyUsageOID.CLIENT_AUTH],
    )


def generate_key_and_signed_cert(
    common_name: str,
    alt_names: list[str],
    valid_days: int,
    extended_key_usages: list = None,
) -> tuple[str, str]:
    """Generate an RSA private key and a certificate signed by the CA.

    extended_key_usages defaults to clientAuth only. Returns (private_key_pem, certificate_pem).
    """
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)
    cert_pem = _issue_leaf(
        private_key.public_key(),
        common_name=common_name,
        alt_names=alt_names,
        valid_days=valid_days,
        extended_key_usages=extended_key_usages or [ExtendedKeyUsageOID.CLIENT_AUTH],
    )

    private_pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")

    return private_pem, cert_pem


def generate_intermediate_ca(cluster_id: str, valid_days: int) -> tuple[str, str]:
    """Generate an RSA private key and an intermediate CA certificate signed by the root CA.

    The intermediate cert has CA:TRUE with path_length=0 (cannot itself sign further CAs)
    and KeyUsage(keyCertSign, cRLSign). No ExtendedKeyUsage is set on the intermediate so
    it can issue both serverAuth and clientAuth leaves.

    The subject is fixed: the cluster marker plus a name made of the cluster id and the
    issue time. The time keeps a replacement's name distinct from the one it replaces:
    while both are trusted, OpenSSL matches certs and CRLs to their issuer by name.

    Returns (private_key_pem, certificate_pem).
    """
    cluster_spiffe_id(cluster_id)  # validates the id
    ca_cert, ca_key = load_ca_material()

    private_key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)
    now = datetime.now(timezone.utc)
    subject = x509.Name(
        [
            x509.NameAttribute(NameOID.ORGANIZATIONAL_UNIT_NAME, CLUSTER_CA_MARKER),
            x509.NameAttribute(
                NameOID.COMMON_NAME, f"Oakestra Cluster CA {cluster_id} {now:%Y-%m-%d %H:%M:%S}"
            ),
        ]
    )

    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(ca_cert.subject)
        .public_key(private_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        # Nothing it issues verifies past the root CA's expiry, so never claim longer.
        .not_valid_after(min(now + timedelta(days=valid_days), ca_cert.not_valid_after_utc))
        .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
        .add_extension(
            x509.KeyUsage(
                digital_signature=False,
                content_commitment=False,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=True,
                crl_sign=True,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .add_extension(
            x509.SubjectKeyIdentifier.from_public_key(private_key.public_key()),
            critical=False,
        )
        .add_extension(
            x509.AuthorityKeyIdentifier.from_issuer_public_key(ca_cert.public_key()),
            critical=False,
        )
    )

    certificate = builder.sign(private_key=ca_key, algorithm=hashes.SHA256())

    private_pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")
    cert_pem = certificate.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    return private_pem, cert_pem


def regenerate_server_files(
    common_name: str,
    alt_names: list[str],
    valid_days: int,
) -> bool:
    """Force-generate a new server key+cert signed by the current CA, overwriting any existing material.

    It always carries the root's identity: it is the root's client cert towards clusters.
    """
    # serverAuth + clientAuth so the same cert can be presented when root calls clusters back.
    private_pem, cert_pem = generate_key_and_signed_cert(
        common_name=common_name,
        alt_names=[ROOT_SPIFFE_ID]
        + [name for name in alt_names if not name.startswith(SPIFFE_PREFIX)],
        valid_days=valid_days,
        extended_key_usages=[ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH],
    )

    write_file_atomic(get_server_key_path(), private_pem, 0o600)
    write_file_atomic(get_server_cert_path(), cert_pem, 0o644)
    return True


def reissue_server_files(valid_days: int = 365) -> bool:
    """Re-sign the server key+cert with the current CA, keeping its name and SANs."""
    cert = x509.load_pem_x509_certificate(get_server_cert_path().read_bytes())
    common_name = cert.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value
    alt_names = cert_alt_names(cert) or [common_name]
    return regenerate_server_files(
        common_name=common_name, alt_names=alt_names, valid_days=valid_days
    )


def ensure_server_files(
    common_name: str,
    alt_names: list[str],
    valid_days: int,
) -> bool:
    """Generate server key+cert only if either is missing. Returns True if generated, False if already present."""
    if get_server_key_path().is_file() and get_server_cert_path().is_file():
        return False
    return regenerate_server_files(
        common_name=common_name, alt_names=alt_names, valid_days=valid_days
    )


def cert_expires_in(cert_pem: str) -> "timedelta":
    """Return how long until the certificate expires (negative if already expired)."""
    cert = x509.load_pem_x509_certificate(cert_pem.encode("utf-8"))
    return cert.not_valid_after_utc - datetime.now(timezone.utc)


def issued_by_root_ca(cert: x509.Certificate) -> bool:
    """Check the cert was signed directly by a trusted root CA.

    A cert issued by a cluster intermediate chains to the root too, but is not accepted here.
    """
    for ca_cert in x509.load_pem_x509_certificates(get_ca_cert_path().read_bytes()):
        try:
            cert.verify_directly_issued_by(ca_cert)
        except (ValueError, TypeError, InvalidSignature):
            continue
        return True
    return False


class ClientCertError(Exception):
    """A forwarded client certificate does not identify an allowed caller."""

    def __init__(self, status: int, message: str):
        super().__init__(message)
        self.status = status


def client_cert_cluster_id(client_cert_header) -> str:
    """The cluster id of the client cert the external gateway verified and forwarded.

    client_cert_header is the URL-encoded PEM the gateway puts in X-Client-Cert. Raises
    ClientCertError unless it is a cluster client cert signed directly by the root CA: the
    gateway accepts any chain to the root CA, so a cert a cluster signed with its own
    intermediate (or the intermediate itself, whose key the cluster holds) gets this far.
    """
    try:
        presented = x509.load_pem_x509_certificate(
            unquote(client_cert_header or "").encode("utf-8")
        )
    except ValueError:
        raise ClientCertError(401, "Client certificate required")
    if is_ca_cert(presented):
        raise ClientCertError(401, "A CA certificate is not a client certificate")
    if not issued_by_root_ca(presented):
        raise ClientCertError(401, "Client certificate was not issued by the root CA")
    cluster_id = cert_cluster_id(presented)
    if cluster_id is None:
        raise ClientCertError(403, "Client certificate is not a cluster certificate")
    return cluster_id


def is_ca_cert(cert: x509.Certificate) -> bool:
    try:
        return cert.extensions.get_extension_for_class(x509.BasicConstraints).value.ca
    except x509.ExtensionNotFound:
        return False


def cert_alt_names(cert: x509.Certificate) -> list[str]:
    """The cert's DNS, IP and URI SANs as strings, in the form _build_san accepts."""
    try:
        san = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    except x509.ExtensionNotFound:
        return []
    return [
        str(name.value)
        for name in san
        if isinstance(name, (x509.DNSName, x509.IPAddress, x509.UniformResourceIdentifier))
    ]


def get_crl_path() -> Path:
    return get_cert_path() / "revoked.crl"


def generate_root_crl(revoked_serials: list, ca_cert=None, ca_key=None) -> bytes:
    """Build and sign a CRL from a list of (serial_int, revoked_at) tuples.

    revoked_serials may be empty — an empty CRL is valid and signals that
    revocation checking is active with nothing currently revoked. Signed by the
    current CA unless ca_cert/ca_key are given. Returns PEM bytes.
    """
    if ca_cert is None or ca_key is None:
        ca_cert, ca_key = load_ca_material()
    now = datetime.now(timezone.utc)

    builder = (
        x509.CertificateRevocationListBuilder()
        .issuer_name(ca_cert.subject)
        .last_update(now)
        .next_update(now + timedelta(days=30))
    )
    for serial, revoked_at in revoked_serials:
        if revoked_at.tzinfo is None:
            revoked_at = revoked_at.replace(tzinfo=timezone.utc)
        revoked = (
            x509.RevokedCertificateBuilder()
            .serial_number(serial)
            .revocation_date(revoked_at)
            .build()
        )
        builder = builder.add_revoked_certificate(revoked)

    crl = builder.sign(private_key=ca_key, algorithm=hashes.SHA256())
    pem = crl.public_bytes(serialization.Encoding.PEM)
    check_crl(pem, ca_cert)
    return pem


def check_crl(crl_pem: bytes, ca_cert: x509.Certificate) -> None:
    """Raise ValueError unless crl_pem is a current CRL that ca_cert verifies."""
    crl = x509.load_pem_x509_crl(crl_pem)
    if crl.issuer != ca_cert.subject or not crl.is_signature_valid(ca_cert.public_key()):
        raise ValueError(f"CRL does not verify against CA '{ca_cert.subject.rfc4514_string()}'")
    if crl.next_update_utc is None or crl.next_update_utc <= datetime.now(timezone.utc):
        raise ValueError("CRL is already expired")


def regenerate_root_crl(revoked_serials: list) -> bool:
    """Regenerate /certs/revoked.crl from the given serial list. Returns True on success.

    During a rotation grace period a second CRL signed by the old CA is appended:
    with CRL checks on, OpenSSL rejects a cert whose issuer has no CRL in the file.
    On failure the previous CRL file is left in place.
    """
    try:
        pem = generate_root_crl(revoked_serials)
        if get_old_ca_cert_path().is_file() and get_old_ca_key_path().is_file():
            old_ca_cert = x509.load_pem_x509_certificate(get_old_ca_cert_path().read_bytes())
            old_ca_key = serialization.load_pem_private_key(
                get_old_ca_key_path().read_bytes(), password=None
            )
            pem += generate_root_crl(revoked_serials, old_ca_cert, old_ca_key)
        write_file_atomic(get_crl_path(), pem, 0o644)
        return True
    except Exception as exc:
        import logging as _logging

        _logging.getLogger(__name__).error("Failed to write CRL to %s: %s", get_crl_path(), exc)
        return False
