import ipaddress
import os
from datetime import datetime, timedelta, timezone
from pathlib import Path

from config import CLUSTER_CA_CERT_FILE, CLUSTER_CA_KEY_FILE, ROOT_CA_FILE
from cryptography import x509
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

DEFAULT_KEY_SIZE = 3072


def load_cluster_ca():
    """Returns (intermediate_cert, intermediate_key, root_ca_cert).
    Raises FileNotFoundError if any of the three files are missing.
    """
    for path in (CLUSTER_CA_CERT_FILE, CLUSTER_CA_KEY_FILE, ROOT_CA_FILE):
        if not path or not Path(path).is_file():
            raise FileNotFoundError("Cluster intermediate CA material is missing")

    intermediate_cert = x509.load_pem_x509_certificate(Path(CLUSTER_CA_CERT_FILE).read_bytes())
    intermediate_key = serialization.load_pem_private_key(
        Path(CLUSTER_CA_KEY_FILE).read_bytes(), password=None
    )
    root_ca_cert = x509.load_pem_x509_certificate(Path(ROOT_CA_FILE).read_bytes())
    return intermediate_cert, intermediate_key, root_ca_cert


def get_root_ca_pem() -> str:
    if not ROOT_CA_FILE or not Path(ROOT_CA_FILE).is_file():
        raise FileNotFoundError("Root CA file is missing")
    return Path(ROOT_CA_FILE).read_text()


def _normalize_pem(pem_data: str, expected_type: str) -> str:
    pem_str = pem_data.strip() if isinstance(pem_data, str) else pem_data.decode("utf-8").strip()
    begin_marker = f"-----BEGIN {expected_type}-----"
    end_marker = f"-----END {expected_type}-----"
    if begin_marker not in pem_str or end_marker not in pem_str:
        raise ValueError(
            f"Invalid PEM format. Expected {expected_type}. "
            f"PEM must contain '{begin_marker}' and '{end_marker}' markers."
        )
    return pem_str


def _build_san(alt_names):
    san_list = []
    for name in alt_names:
        try:
            san_list.append(x509.IPAddress(ipaddress.ip_address(name)))
        except ValueError:
            san_list.append(x509.DNSName(name))
    return x509.SubjectAlternativeName(san_list)


def _sign_with_intermediate(builder, intermediate_key):
    return builder.sign(private_key=intermediate_key, algorithm=hashes.SHA256())


def generate_worker_cert(
    common_name: str, alt_names=None, valid_days: int = 365
) -> tuple[str, str]:
    """Generate an RSA private key and a worker leaf certificate signed by the cluster intermediate CA.

    Returns (private_key_pem, fullchain_pem) where fullchain is leaf || intermediate.
    """
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)
    fullchain_pem = _issue_leaf(private_key.public_key(), common_name, alt_names, valid_days)
    private_pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")
    return private_pem, fullchain_pem


def _issue_leaf(public_key, common_name: str, alt_names, valid_days: int) -> str:
    """Sign a client+server leaf for public_key with the intermediate. Returns leaf || intermediate."""
    intermediate_cert, intermediate_key, _ = load_cluster_ca()

    now = datetime.now(timezone.utc)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)])

    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(intermediate_cert.subject)
        .public_key(public_key)
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        # Nothing verifies past the intermediate's expiry, so never claim longer.
        .not_valid_after(
            min(now + timedelta(days=valid_days), intermediate_cert.not_valid_after_utc)
        )
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
        .add_extension(
            x509.ExtendedKeyUsage(
                [
                    x509.oid.ExtendedKeyUsageOID.CLIENT_AUTH,
                    x509.oid.ExtendedKeyUsageOID.SERVER_AUTH,
                ]
            ),
            critical=False,
        )
        .add_extension(
            x509.SubjectKeyIdentifier.from_public_key(public_key),
            critical=False,
        )
        .add_extension(
            x509.AuthorityKeyIdentifier.from_issuer_public_key(intermediate_cert.public_key()),
            critical=False,
        )
    )
    if alt_names:
        builder = builder.add_extension(_build_san(alt_names), critical=False)

    leaf_cert = _sign_with_intermediate(builder, intermediate_key)

    leaf_pem = leaf_cert.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    intermediate_pem = intermediate_cert.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    return leaf_pem + intermediate_pem


def cert_expires_in(cert_pem: str) -> timedelta:
    """Return how long until the certificate expires (negative if already expired)."""
    cert = x509.load_pem_x509_certificate(cert_pem.encode("utf-8"))
    return cert.not_valid_after_utc - datetime.now(timezone.utc)


def cert_issued_by(cert_pem: str, ca_pem: str) -> bool:
    """Return True if the certificate's signature was made by the given CA's key."""
    cert = x509.load_pem_x509_certificate(cert_pem.encode("utf-8"))
    ca_cert = x509.load_pem_x509_certificate(ca_pem.encode("utf-8"))
    try:
        cert.verify_directly_issued_by(ca_cert)
    except (ValueError, TypeError, InvalidSignature):
        return False
    return True


def get_cluster_crl_path() -> Path:
    base = Path(ROOT_CA_FILE).parent if ROOT_CA_FILE else Path("/certs")
    return base / "cluster_revoked.crl"


def generate_cluster_crl(revoked_serials: list, intermediate=None) -> bytes:
    """Build and sign a CRL from (serial_int, revoked_at) tuples using a cluster intermediate CA.

    Signed by the current intermediate unless intermediate=(cert, key) is given. An empty
    revoked_serials list produces a valid empty CRL. Returns PEM bytes.
    """
    if intermediate is None:
        intermediate_cert, intermediate_key, _ = load_cluster_ca()
    else:
        intermediate_cert, intermediate_key = intermediate
    now = datetime.now(timezone.utc)

    builder = (
        x509.CertificateRevocationListBuilder()
        .issuer_name(intermediate_cert.subject)
        .last_update(now)
        .next_update(now + timedelta(days=30))
        # Lets OpenSSL pick the right CRL when two intermediates are trusted.
        .add_extension(
            x509.AuthorityKeyIdentifier.from_issuer_public_key(intermediate_cert.public_key()),
            critical=False,
        )
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

    crl = builder.sign(private_key=intermediate_key, algorithm=hashes.SHA256())
    return crl.public_bytes(serialization.Encoding.PEM)


def regenerate_cluster_crl(revoked_serials: list) -> bool:
    """Regenerate /certs/cluster_revoked.crl from the given serial list. Returns True on success.

    While workers migrate to a new intermediate, a second CRL signed by the old one is
    appended: mosquitto rejects a cert whose issuer has no CRL in the file.
    """
    try:
        pem = generate_cluster_crl(revoked_serials)
        old = load_old_cluster_ca()
        if old is not None:
            pem += generate_cluster_crl(revoked_serials, old)
        crl_path = get_cluster_crl_path()
        crl_path.write_bytes(pem)
        os.chmod(crl_path, 0o644)
        return True
    except Exception:
        return False


def _cluster_ca_dir() -> Path:
    return Path(CLUSTER_CA_CERT_FILE).parent if CLUSTER_CA_CERT_FILE else Path("/certs")


def get_old_cluster_ca_paths() -> tuple[Path, Path, Path]:
    """The previous intermediate's cert, key and grace deadline, kept while workers migrate."""
    base = _cluster_ca_dir()
    return base / "cluster_ca.old.crt", base / "cluster_ca.old.key", base / "cluster_ca.old.expiry"


def load_old_cluster_ca():
    """Return (cert, key) of the previous intermediate during a migration, else None."""
    cert_path, key_path, _ = get_old_cluster_ca_paths()
    if not (cert_path.is_file() and key_path.is_file()):
        return None
    cert = x509.load_pem_x509_certificate(cert_path.read_bytes())
    key = serialization.load_pem_private_key(key_path.read_bytes(), password=None)
    return cert, key


def issued_by_cluster_ca(cert) -> bool:
    """Check the signature against intermediate CA"""
    candidates = [Path(CLUSTER_CA_CERT_FILE).read_text()]
    old = load_old_cluster_ca()
    if old is not None:
        candidates.append(old[0].public_bytes(serialization.Encoding.PEM).decode("utf-8"))
    cert_pem = cert.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    return any(cert_issued_by(cert_pem, ca_pem) for ca_pem in candidates)


def key_id(cert) -> str:
    """Hex key identifier of a CA cert, as found in the AKI of the certs it issues."""
    return x509.SubjectKeyIdentifier.from_public_key(cert.public_key()).digest.hex()


_key_id_cache = {}


def cluster_ca_key_ids() -> tuple[str, str | None]:
    """Key IDs of the current and (during a migration) previous intermediate.

    Called for every worker heartbeat, so the result is cached per file version.
    """
    old_cert_path = get_old_cluster_ca_paths()[0]
    stamp = (
        os.stat(CLUSTER_CA_CERT_FILE).st_mtime_ns,
        old_cert_path.stat().st_mtime_ns if old_cert_path.is_file() else None,
    )
    if _key_id_cache.get("stamp") != stamp:
        current = x509.load_pem_x509_certificate(Path(CLUSTER_CA_CERT_FILE).read_bytes())
        old = None
        if old_cert_path.is_file():
            old = key_id(x509.load_pem_x509_certificate(old_cert_path.read_bytes()))
        _key_id_cache.update(stamp=stamp, ids=(key_id(current), old))
    return _key_id_cache["ids"]


def get_cluster_mqtt_identity_paths() -> tuple[Path, Path]:
    base = Path(ROOT_CA_FILE).parent if ROOT_CA_FILE else Path("/certs")
    return base / "cluster_mqtt.crt", base / "cluster_mqtt.key"


def write_cluster_mqtt_identity(cluster_name: str) -> Path:
    """Issue the MQTT client certificate used by this cluster's own services.

    mosquitto CRL-checks client certificates against cluster_revoked.crl, which only the
    cluster intermediate CA signs, so the root-issued cluster.crt would be rejected as an
    MQTT client certificate. This identity is issued by the intermediate instead.
    Returns the certificate path.
    """
    private_pem, fullchain_pem = generate_worker_cert(f"{cluster_name or 'cluster'}-mqtt")
    return _write_identity(get_cluster_mqtt_identity_paths(), private_pem, fullchain_pem)


def get_mqtt_server_identity_paths() -> tuple[Path, Path]:
    base = Path(ROOT_CA_FILE).parent if ROOT_CA_FILE else Path("/certs")
    return base / "mqtt_server.crt", base / "mqtt_server.key"


def write_mqtt_server_identity(cluster_name: str, cluster_address: str) -> Path:
    """Issue the MQTT broker's server certificate from the cluster intermediate CA."""
    alt_names = [name for name in (cluster_address, "mqtt", "localhost") if name]
    private_pem, fullchain_pem = generate_worker_cert(
        f"{cluster_name or 'cluster'}-mqtt-broker", alt_names
    )
    return _write_identity(get_mqtt_server_identity_paths(), private_pem, fullchain_pem)


def _write_identity(paths: tuple[Path, Path], private_pem: str, fullchain_pem: str) -> Path:
    cert_path, key_path = paths
    key_path.write_text(private_pem)
    os.chmod(key_path, 0o600)
    cert_path.write_text(fullchain_pem)
    os.chmod(cert_path, 0o644)
    return cert_path


def generate_cluster_csr(common_name: str, alt_names: list) -> tuple:
    """Generate a new RSA private key and CSR for cluster client cert renewal.

    The CSR includes clientAuth + serverAuth EKUs so the signed cert can be
    used both as an mTLS client identity (cluster→root) and as a server cert
    (root→cluster callbacks).

    Returns (private_key_pem, csr_pem).
    """
    key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)])

    san_list = []
    for name in alt_names or []:
        try:
            san_list.append(x509.IPAddress(ipaddress.ip_address(name)))
        except ValueError:
            san_list.append(x509.DNSName(name))

    builder = x509.CertificateSigningRequestBuilder().subject_name(subject)
    if san_list:
        builder = builder.add_extension(x509.SubjectAlternativeName(san_list), critical=False)
    builder = builder.add_extension(
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
    builder = builder.add_extension(
        x509.ExtendedKeyUsage([ExtendedKeyUsageOID.CLIENT_AUTH, ExtendedKeyUsageOID.SERVER_AUTH]),
        critical=False,
    )

    csr = builder.sign(key, hashes.SHA256())

    key_pem = key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")
    csr_pem = csr.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    return key_pem, csr_pem


def load_worker_csr(csr_pem: str):
    """Parse a worker CSR and check it was signed by the key it carries."""
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


def csr_common_name(csr) -> str:
    names = csr.subject.get_attributes_for_oid(NameOID.COMMON_NAME)
    if not names or not names[0].value.strip():
        raise ValueError("CSR has no common name")
    return names[0].value


def sign_worker_csr(csr_pem: str, valid_days: int = 365) -> str:
    """Sign a worker CSR with the cluster intermediate CA. Returns fullchain PEM.

    Only the CSR's key, common name and DNS/IP names are used; every other extension
    is set here, so a CSR cannot ask for a CA certificate or other key usages.
    """
    csr = load_worker_csr(csr_pem)
    alt_names = []
    try:
        san = csr.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
        alt_names = [str(name) for name in san.get_values_for_type(x509.DNSName)]
        alt_names += [str(ip) for ip in san.get_values_for_type(x509.IPAddress)]
    except x509.ExtensionNotFound:
        pass
    return _issue_leaf(csr.public_key(), csr_common_name(csr), alt_names or None, valid_days)
