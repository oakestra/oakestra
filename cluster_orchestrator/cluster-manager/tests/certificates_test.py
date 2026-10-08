import ipaddress
import os
import re
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest import mock
from urllib.parse import quote

import bootstrap_certificates
import config
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from ext_requests import certificates_db
from utils import certificates

# ---------------------------------------------------------------------------
# Root gateway trust (ROOT_GATEWAY_TRUST)
# ---------------------------------------------------------------------------


class RootGatewayTrustTestCase(unittest.TestCase):
    def setUp(self):
        handle, self.ca_file = tempfile.mkstemp(suffix=".crt")
        os.write(handle, b"-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n")
        os.close(handle)
        self.addCleanup(os.remove, self.ca_file)

    def configured(self, value):
        return mock.patch.multiple(config, ROOT_GATEWAY_TRUST=value, ROOT_CA_FILE=self.ca_file)


class ParseRootGatewayTrustTest(RootGatewayTrustTestCase):
    def test_empty_uses_internal_ca(self):
        trust = config.parse_root_gateway_trust("", self.ca_file)
        self.assertEqual(trust, config.GatewayTrust(config.TRUST_CA_BUNDLE, self.ca_file))

    def test_empty_without_internal_ca(self):
        with self.assertRaisesRegex(ValueError, "ROOT_CA_FILE is not set"):
            config.parse_root_gateway_trust("", None)

    def test_insecure_does_not_fall_back_to_internal_ca(self):
        trust = config.parse_root_gateway_trust("insecure", self.ca_file)
        self.assertEqual(trust, config.GatewayTrust(config.TRUST_INSECURE))

    def test_missing_path(self):
        with self.assertRaisesRegex(ValueError, "does not exist inside the container"):
            config.parse_root_gateway_trust("/home/someone/root-ca.crt", self.ca_file)


class GrpcRootCertificatesTest(RootGatewayTrustTestCase):
    def test_insecure_is_rejected(self):
        with self.configured("insecure"), self.assertRaisesRegex(ValueError, "gRPC channel"):
            config.root_gateway_grpc_root_certificates()


class BootstrapResolveVerifyTest(RootGatewayTrustTestCase):
    def resolve(self, value):
        with (
            self.configured(value),
            mock.patch.dict(os.environ, {"ROOT_GATEWAY_TRUST": value}),
            mock.patch.object(bootstrap_certificates, "_fetch_root_ca_tofu") as tofu,
        ):
            return bootstrap_certificates._resolve_verify("https://root:443"), tofu

    def test_empty_fetches_internal_ca_then_uses_it(self):
        verify, tofu = self.resolve("")
        self.assertEqual(verify, self.ca_file)
        tofu.assert_called_once()

    def test_missing_path_exits(self):
        with self.assertRaises(SystemExit):
            self.resolve("/does/not/exist.crt")


# ---------------------------------------------------------------------------
# Cluster intermediate CA
# ---------------------------------------------------------------------------


CLUSTER_ID = "65d200f3812caeb85e21ee19"
OTHER_CLUSTER_ID = "65d200f3812caeb85e21ee12"
NODE_ID = "65d200f3812caeb85e21ee55"


def make_cert(name, issuer=None, days=365, uri=None, ca=True):
    """Return (cert, key); self-signed unless issuer=(cert, key) is given."""
    key = ec.generate_private_key(ec.SECP256R1())
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, name)])
    issuer_cert, issuer_key = issuer or (None, key)
    now = datetime.now(timezone.utc)
    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer_cert.subject if issuer_cert else subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(days=days))
        .add_extension(x509.BasicConstraints(ca=ca, path_length=None), critical=True)
    )
    if uri:
        builder = builder.add_extension(
            x509.SubjectAlternativeName([x509.UniformResourceIdentifier(uri)]), critical=False
        )
    return builder.sign(issuer_key, hashes.SHA256()), key


def worker_uri(cluster_id=CLUSTER_ID, node_id=NODE_ID):
    return f"spiffe://oakestra/cluster/{cluster_id}/worker/{node_id}"


def uris(cert):
    san = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    return san.get_values_for_type(x509.UniformResourceIdentifier)


def usages(cert):
    return list(cert.extensions.get_extension_for_class(x509.ExtendedKeyUsage).value)


def make_csr(common_name=None, extensions=()):
    key = ec.generate_private_key(ec.SECP256R1())
    attributes = [x509.NameAttribute(NameOID.COMMON_NAME, common_name)] if common_name else []
    builder = x509.CertificateSigningRequestBuilder().subject_name(x509.Name(attributes))
    for extension in extensions:
        builder = builder.add_extension(extension, critical=False)
    return builder.sign(key, hashes.SHA256()).public_bytes(serialization.Encoding.PEM).decode()


def write_ca(cert_path, key_path, ca):
    cert, key = ca
    Path(cert_path).write_bytes(cert.public_bytes(serialization.Encoding.PEM))
    Path(key_path).write_bytes(
        key.private_bytes(
            serialization.Encoding.PEM,
            serialization.PrivateFormat.PKCS8,
            serialization.NoEncryption(),
        )
    )


class ClusterCaTestCase(unittest.TestCase):
    """A root CA and a cluster intermediate CA, written where utils.certificates looks."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.dir = Path(tmp.name)

        self.root = make_cert("root")
        self.intermediate = make_cert("cluster", self.root, days=30)
        write_ca(self.dir / "cluster_ca.crt", self.dir / "cluster_ca.key", self.intermediate)
        write_ca(self.dir / "root.crt", self.dir / "root.key", self.root)
        # The cluster's own client cert, which carries its id.
        cluster_cert = make_cert(
            CLUSTER_ID, self.root, uri=f"spiffe://oakestra/cluster/{CLUSTER_ID}", ca=False
        )
        write_ca(self.dir / "cluster.crt", self.dir / "cluster.key", cluster_cert)

        patcher = mock.patch.multiple(
            certificates,
            CLUSTER_CA_CERT_FILE=str(self.dir / "cluster_ca.crt"),
            CLUSTER_CA_KEY_FILE=str(self.dir / "cluster_ca.key"),
            CLUSTER_CERT_FILE=str(self.dir / "cluster.crt"),
            ROOT_CA_FILE=str(self.dir / "root.crt"),
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def start_migration(self):
        """Keep a previous intermediate next to the current one, as a rotation does."""
        old = make_cert("old-cluster", self.root)
        write_ca(self.dir / "cluster_ca.old.crt", self.dir / "cluster_ca.old.key", old)
        return old


class SignWorkerCsrTest(ClusterCaTestCase):
    def test_csr_cannot_choose_identity_ca_or_usages(self):
        csr = make_csr(
            "other-worker",
            [
                x509.BasicConstraints(ca=True, path_length=None),
                x509.KeyUsage(
                    digital_signature=True,
                    content_commitment=False,
                    key_encipherment=False,
                    data_encipherment=False,
                    key_agreement=False,
                    key_cert_sign=True,
                    crl_sign=True,
                    encipher_only=False,
                    decipher_only=False,
                ),
                x509.ExtendedKeyUsage(
                    [ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CODE_SIGNING]
                ),
                x509.SubjectAlternativeName(
                    [
                        x509.DNSName("mqtt"),
                        x509.IPAddress(ipaddress.ip_address("10.0.0.5")),
                        x509.UniformResourceIdentifier("spiffe://oakestra/root"),
                    ]
                ),
            ],
        )

        leaf, chain_ca = x509.load_pem_x509_certificates(
            certificates.sign_worker_csr(csr, NODE_ID).encode()
        )

        extensions = leaf.extensions
        self.assertFalse(extensions.get_extension_for_class(x509.BasicConstraints).value.ca)
        key_usage = extensions.get_extension_for_class(x509.KeyUsage).value
        self.assertFalse(key_usage.key_cert_sign)
        self.assertFalse(key_usage.crl_sign)
        # A worker is only ever a client: it must not be able to pose as the broker.
        self.assertEqual(usages(leaf), [ExtendedKeyUsageOID.CLIENT_AUTH])
        # The issuer names the worker; the CSR's names are ignored.
        self.assertEqual(leaf.subject.rfc4514_string(), f"CN={NODE_ID}")
        san = extensions.get_extension_for_class(x509.SubjectAlternativeName).value
        self.assertEqual(list(san), [x509.UniformResourceIdentifier(worker_uri())])
        leaf.verify_directly_issued_by(self.intermediate[0])
        self.assertEqual(chain_ca, self.intermediate[0])

    def test_validity_is_capped_at_intermediate_expiry(self):
        fullchain = certificates.sign_worker_csr(make_csr("worker-1"), NODE_ID, valid_days=365)
        leaf = x509.load_pem_x509_certificate(fullchain.encode())
        self.assertLessEqual(leaf.not_valid_after_utc, self.intermediate[0].not_valid_after_utc)

    def test_invalid_node_id_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "Invalid node id"):
            certificates.sign_worker_csr(make_csr("worker-1"), "../root")


class ClusterIdentityTest(ClusterCaTestCase):
    def test_own_cluster_id_comes_from_the_cluster_cert(self):
        self.assertEqual(certificates.own_cluster_id(), CLUSTER_ID)

    def test_worker_ids(self):
        worker = make_cert("w", self.intermediate, uri=worker_uri(), ca=False)[0]
        self.assertEqual(certificates.cert_worker_ids(worker), (CLUSTER_ID, NODE_ID))

    def test_other_identities_are_not_workers(self):
        for uri in (
            f"spiffe://oakestra/cluster/{CLUSTER_ID}",
            f"spiffe://oakestra/cluster/{CLUSTER_ID}/mqtt-client",
            "spiffe://oakestra/root",
            worker_uri() + "/extra",
        ):
            cert = make_cert("x", self.intermediate, uri=uri, ca=False)[0]
            self.assertIsNone(certificates.cert_worker_ids(cert), uri)

    def test_mqtt_client_is_client_only(self):
        cert_path = certificates.write_cluster_mqtt_identity()
        cert = x509.load_pem_x509_certificate(cert_path.read_bytes())
        self.assertEqual(usages(cert), [ExtendedKeyUsageOID.CLIENT_AUTH])
        self.assertEqual(uris(cert), [f"spiffe://oakestra/cluster/{CLUSTER_ID}/mqtt-client"])

    def test_mqtt_broker_is_server_only(self):
        cert_path = certificates.write_mqtt_server_identity("10.0.0.1")
        cert = x509.load_pem_x509_certificate(cert_path.read_bytes())
        self.assertEqual(usages(cert), [ExtendedKeyUsageOID.SERVER_AUTH])
        san = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
        self.assertEqual(
            san.get_values_for_type(x509.UniformResourceIdentifier),
            [f"spiffe://oakestra/cluster/{CLUSTER_ID}/mqtt-broker"],
        )
        self.assertEqual(san.get_values_for_type(x509.DNSName), ["mqtt", "localhost"])
        self.assertEqual(
            san.get_values_for_type(x509.IPAddress), [ipaddress.ip_address("10.0.0.1")]
        )


class ClientCertWorkerTest(ClusterCaTestCase):
    """The checks behind /api/node/register and /api/certs/worker-renew."""

    def verify(self, cert):
        return certificates.client_cert_worker(
            quote(cert.public_bytes(serialization.Encoding.PEM).decode())
        )

    def assert_rejected(self, cert, status):
        with self.assertRaises(certificates.ClientCertError) as raised:
            self.verify(cert)
        self.assertEqual(raised.exception.status, status)

    def test_accepts_a_worker_of_this_cluster(self):
        worker = make_cert("w", self.intermediate, uri=worker_uri(), ca=False)[0]
        self.assertEqual(self.verify(worker)[1], NODE_ID)

    def test_rejects_a_worker_of_another_cluster(self):
        # Chains to the root, so the gateway lets it through.
        other_intermediate = make_cert("other-cluster", self.root)
        worker = make_cert("w", other_intermediate, uri=worker_uri(OTHER_CLUSTER_ID), ca=False)[0]
        self.assert_rejected(worker, 401)

    def test_rejects_a_root_issued_cert(self):
        cluster = make_cert("c", self.root, uri=f"spiffe://oakestra/cluster/{CLUSTER_ID}")[0]
        self.assert_rejected(cluster, 401)

    def test_rejects_a_foreign_worker_id_from_this_intermediate(self):
        worker = make_cert("w", self.intermediate, uri=worker_uri(OTHER_CLUSTER_ID), ca=False)[0]
        self.assert_rejected(worker, 403)

    def test_rejects_non_worker_identities(self):
        mqtt_client = make_cert(
            "m", self.intermediate, uri=f"spiffe://oakestra/cluster/{CLUSTER_ID}/mqtt-client"
        )[0]
        self.assert_rejected(mqtt_client, 403)

    def test_requires_a_client_cert(self):
        with self.assertRaises(certificates.ClientCertError) as raised:
            certificates.client_cert_worker(None)
        self.assertEqual(raised.exception.status, 401)


class IssuedByClusterCaTest(ClusterCaTestCase):
    def test_accepts_current_and_old_intermediate(self):
        old = self.start_migration()
        self.assertTrue(certificates.issued_by_cluster_ca(make_cert("w1", self.intermediate)[0]))
        self.assertTrue(certificates.issued_by_cluster_ca(make_cert("w2", old)[0]))

    def test_rejects_other_cluster_under_same_root(self):
        # Chains to the same root, so the gateway accepts it; this cluster must not.
        other_cluster = make_cert("other-cluster", self.root)
        worker = make_cert("w1", other_cluster)[0]
        self.assertFalse(certificates.issued_by_cluster_ca(worker))


class ClusterCrlTest(ClusterCaTestCase):
    def test_migration_writes_a_crl_per_intermediate(self):
        # mosquitto rejects a cert whose issuer has no CRL in the file.
        old = self.start_migration()
        self.assertTrue(certificates.regenerate_cluster_crl([(1234, datetime.now(timezone.utc))]))

        pems = re.findall(
            rb"-----BEGIN X509 CRL-----.+?-----END X509 CRL-----",
            (self.dir / "cluster_revoked.crl").read_bytes(),
            re.DOTALL,
        )
        crls = [x509.load_pem_x509_crl(pem) for pem in pems]
        self.assertEqual(len(crls), 2)
        for crl, (ca, _) in zip(crls, (self.intermediate, old)):
            self.assertTrue(crl.is_signature_valid(ca.public_key()))
            self.assertIsNotNone(crl.get_revoked_certificate_by_serial_number(1234))


# ---------------------------------------------------------------------------
# Worker registration tokens
# ---------------------------------------------------------------------------


class FakeCollection:
    """Enough of a pymongo collection for token redemption. Like Mongo, it hands
    datetimes back without a timezone."""

    def __init__(self):
        self.docs = []

    def insert_one(self, doc):
        self.docs.append(
            {k: v.replace(tzinfo=None) if isinstance(v, datetime) else v for k, v in doc.items()}
        )

    def find_one_and_delete(self, query):
        for doc in self.docs:
            if all(doc.get(k) == v for k, v in query.items()):
                self.docs.remove(doc)
                return doc
        return None


class ConsumeTokenTest(unittest.TestCase):
    def setUp(self):
        self.tokens = FakeCollection()
        patcher = mock.patch.object(
            certificates_db, "_worker_tokens_collection", return_value=self.tokens
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def store(self, token, expires_in):
        expiry = datetime.now(timezone.utc) + expires_in
        certificates_db.store_token_hash(certificates_db.hash_worker_token(token), expiry)

    def test_token_is_single_use(self):
        self.store("token", timedelta(minutes=10))
        self.assertIsNotNone(certificates_db.consume_token("token"))
        self.assertIsNone(certificates_db.consume_token("token"))

    def test_minted_token_is_redeemable_once(self):
        token, expiry = certificates_db.create_worker_token(10)
        self.assertGreater(expiry, datetime.now(timezone.utc))
        self.assertNotIn(token, str(self.tokens.docs))
        self.assertIsNotNone(certificates_db.consume_token(token))
        self.assertIsNone(certificates_db.consume_token(token))

    def test_expired_token_is_rejected_and_removed(self):
        self.store("token", timedelta(minutes=-1))
        self.assertIsNone(certificates_db.consume_token("token"))
        self.assertEqual(self.tokens.docs, [])

    def test_token_carries_the_node_id_it_reprovisions(self):
        token, _ = certificates_db.create_worker_token(10, node_id=NODE_ID)
        self.assertEqual(certificates_db.consume_token(token)["node_id"], NODE_ID)

    def test_new_worker_token_has_no_node_id(self):
        token, _ = certificates_db.create_worker_token(10)
        self.assertNotIn("node_id", certificates_db.consume_token(token))


if __name__ == "__main__":
    unittest.main()
