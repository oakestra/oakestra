import ipaddress
from datetime import datetime, timedelta, timezone
from pathlib import Path

from config import CLUSTER_CA_CERT_FILE, CLUSTER_CA_KEY_FILE, ROOT_CA_FILE
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtensionOID, NameOID

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
    intermediate_cert, intermediate_key, _ = load_cluster_ca()

    private_key = rsa.generate_private_key(public_exponent=65537, key_size=DEFAULT_KEY_SIZE)
    now = datetime.now(timezone.utc)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)])

    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(intermediate_cert.subject)
        .public_key(private_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(days=valid_days))
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
            x509.SubjectKeyIdentifier.from_public_key(private_key.public_key()),
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
    fullchain_pem = leaf_pem + intermediate_pem

    private_pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")

    return private_pem, fullchain_pem


def sign_worker_csr(csr_pem: str, valid_days: int = 365) -> str:
    """Sign an externally-provided CSR with the cluster intermediate CA.
    Returns fullchain PEM (leaf || intermediate).
    """
    intermediate_cert, intermediate_key, _ = load_cluster_ca()

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
        .issuer_name(intermediate_cert.subject)
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

    leaf_cert = _sign_with_intermediate(builder, intermediate_key)
    leaf_pem = leaf_cert.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    intermediate_pem = intermediate_cert.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    return leaf_pem + intermediate_pem
