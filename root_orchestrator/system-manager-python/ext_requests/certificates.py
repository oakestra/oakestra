import ipaddress
import os
from datetime import datetime, timedelta, timezone
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtensionOID, NameOID

DEFAULT_CERT_PATH = "/certs"
DEFAULT_CA_COMMON_NAME = "Oakestra Root CA"
DEFAULT_CA_VALID_DAYS = 365
DEFAULT_KEY_SIZE = 3072

KONG_CA_CERT_UUID = "cafe0000-0000-4000-8000-000000000000"


def get_cert_path() -> Path:
    cert_path = Path(os.environ.get("CERT_PATH") or DEFAULT_CERT_PATH)
    cert_path.mkdir(parents=True, exist_ok=True)
    return cert_path


def get_ca_key_path() -> Path:
    return get_cert_path() / "ca.key"


def get_ca_cert_path() -> Path:
    return get_cert_path() / "ca.crt"


def get_server_key_path() -> Path:
    return get_cert_path() / "server.key"


def get_server_cert_path() -> Path:
    return get_cert_path() / "server.crt"


def ca_files_exist() -> bool:
    return get_ca_key_path().is_file() and get_ca_cert_path().is_file()


def regenerate_ca_files(
    valid_days: int,
    common_name: str = DEFAULT_CA_COMMON_NAME,
) -> bool:
    """Force-generate a new CA key+cert, overwriting any existing material."""
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)
    now = datetime.now(timezone.utc)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)])

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
    get_ca_key_path().write_bytes(
        private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.TraditionalOpenSSL,
            encryption_algorithm=serialization.NoEncryption(),
        )
    )
    os.chmod(get_ca_key_path(), 0o600)
    get_ca_cert_path().write_bytes(certificate.public_bytes(serialization.Encoding.PEM))
    os.chmod(get_ca_cert_path(), 0o644)
    return True


def ensure_ca_files(
    common_name: str = DEFAULT_CA_COMMON_NAME,
    valid_days: int = DEFAULT_CA_VALID_DAYS,
) -> bool:
    """Generate CA key+cert only if they are missing. Returns True if generated, False if already present."""
    if ca_files_exist():
        return False
    return regenerate_ca_files(common_name=common_name, valid_days=valid_days)


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


def sign_csr_pem(csr_pem: str, valid_days: int) -> str:
    ca_cert, ca_key = load_ca_material()

    try:
        normalized_csr = _normalize_pem(csr_pem, "CERTIFICATE REQUEST")
        csr = x509.load_pem_x509_csr(normalized_csr.encode("utf-8"))
    except ValueError as ve:
        raise ValueError(f"CSR parsing failed: {str(ve)}")
    except Exception as e:
        raise ValueError(f"Failed to load CSR: {str(e)}")
    now = datetime.now(timezone.utc)

    builder = (
        x509.CertificateBuilder()
        .subject_name(csr.subject)
        .issuer_name(ca_cert.subject)
        .public_key(csr.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(days=valid_days))
    )

    csr_extension_oids = {extension.oid for extension in csr.extensions}
    if ExtensionOID.BASIC_CONSTRAINTS not in csr_extension_oids:
        builder = builder.add_extension(
            x509.BasicConstraints(ca=False, path_length=None), critical=True
        )

    for extension in csr.extensions:
        builder = builder.add_extension(extension.value, extension.critical)

    certificate = builder.sign(private_key=ca_key, algorithm=hashes.SHA256())
    return certificate.public_bytes(serialization.Encoding.PEM).decode("utf-8")


def generate_key_and_signed_cert(
    common_name: str,
    alt_names: list[str],
    valid_days: int,
    extended_key_usages: list = None,
) -> tuple[str, str]:
    """Generate an RSA private key and a certificate signed by the CA.

    Returns (private_key_pem, certificate_pem).
    """
    # Ensure CA exists
    ca_cert, ca_key = load_ca_material()

    # Generate private key
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)

    # Build subject
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)])

    # Build CSR with optional SANs
    csr_builder = x509.CertificateSigningRequestBuilder().subject_name(subject)
    if alt_names:
        san_list = []
        for name in alt_names:
            try:
                san_list.append(x509.IPAddress(ipaddress.ip_address(name)))
            except ValueError:
                san_list.append(x509.DNSName(name))
        csr_builder = csr_builder.add_extension(
            x509.SubjectAlternativeName(san_list), critical=False
        )

    if extended_key_usages:
        csr_builder = csr_builder.add_extension(
            x509.ExtendedKeyUsage(extended_key_usages), critical=False
        )

    csr = csr_builder.sign(private_key, hashes.SHA256())

    # Sign CSR using CA material via existing sign function
    csr_pem = csr.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    cert_pem = sign_csr_pem(csr_pem=csr_pem, valid_days=valid_days)

    private_pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")

    return private_pem, cert_pem


def generate_intermediate_ca(
    common_name: str,
    alt_names: list[str],
    valid_days: int,
) -> tuple[str, str]:
    """Generate an RSA private key and an intermediate CA certificate signed by the root CA.

    The intermediate cert has CA:TRUE with path_length=0 (cannot itself sign further CAs)
    and KeyUsage(keyCertSign, cRLSign). No ExtendedKeyUsage is set on the intermediate so
    it can issue both serverAuth and clientAuth leaves.

    Returns (private_key_pem, certificate_pem).
    """
    ca_cert, ca_key = load_ca_material()

    private_key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)
    now = datetime.now(timezone.utc)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)])

    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(ca_cert.subject)
        .public_key(private_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(days=valid_days))
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

    if alt_names:
        san_list = []
        for name in alt_names:
            try:
                san_list.append(x509.IPAddress(ipaddress.ip_address(name)))
            except ValueError:
                san_list.append(x509.DNSName(name))
        builder = builder.add_extension(x509.SubjectAlternativeName(san_list), critical=False)

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
    """Force-generate a new server key+cert signed by the current CA, overwriting any existing material."""
    # serverAuth + clientAuth so the same cert can be presented when root calls clusters back.
    private_pem, cert_pem = generate_key_and_signed_cert(
        common_name=common_name,
        alt_names=alt_names,
        valid_days=valid_days,
        extended_key_usages=[
            x509.oid.ExtendedKeyUsageOID.SERVER_AUTH,
            x509.oid.ExtendedKeyUsageOID.CLIENT_AUTH,
        ],
    )

    get_server_key_path().write_text(private_pem)
    os.chmod(get_server_key_path(), 0o600)
    get_server_cert_path().write_text(cert_pem)
    os.chmod(get_server_cert_path(), 0o644)
    return True


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
