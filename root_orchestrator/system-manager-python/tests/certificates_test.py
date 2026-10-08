import ipaddress
import os
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from unittest import mock
from urllib.parse import quote

from blueprints import certificates_blueprints, clusters_blueprints
from blueprints.certificates_blueprints import ClusterCertRenewController
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from flask import Flask
from utils import certificates
from werkzeug.exceptions import HTTPException

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

CLUSTER_ID = "65d200f3812caeb85e21ee19"
OTHER_CLUSTER_ID = "65d200f3812caeb85e21ee12"
CLUSTER_URI = f"spiffe://oakestra/cluster/{CLUSTER_ID}"


def make_csr(common_name=None, extensions=()):
    key = ec.generate_private_key(ec.SECP256R1())
    attributes = [x509.NameAttribute(NameOID.COMMON_NAME, common_name)] if common_name else []
    builder = x509.CertificateSigningRequestBuilder().subject_name(x509.Name(attributes))
    for extension in extensions:
        builder = builder.add_extension(extension, critical=False)
    return builder.sign(key, hashes.SHA256()).public_bytes(serialization.Encoding.PEM).decode()


# Everything a CSR could ask for that the root must not grant.
GREEDY_EXTENSIONS = [
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
    x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CODE_SIGNING]),
    x509.SubjectAlternativeName(
        [
            x509.DNSName("mqtt"),
            x509.IPAddress(ipaddress.ip_address("10.0.0.5")),
            x509.UniformResourceIdentifier("spiffe://oakestra/root"),
        ]
    ),
]


class RootCaTestCase(unittest.TestCase):
    """A root CA written to a temporary CERT_PATH, where utils.certificates looks."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        patcher = mock.patch.dict(os.environ, {"CERT_PATH": tmp.name})
        patcher.start()
        self.addCleanup(patcher.stop)
        certificates.regenerate_ca_files(valid_days=30)
        self.root_cert, _ = certificates.load_ca_material()

    def assert_client_leaf(self, cert):
        """A clientAuth-only end-entity cert issued directly by the root CA."""
        extensions = cert.extensions
        self.assertFalse(extensions.get_extension_for_class(x509.BasicConstraints).value.ca)
        key_usage = extensions.get_extension_for_class(x509.KeyUsage).value
        self.assertFalse(key_usage.key_cert_sign)
        self.assertFalse(key_usage.crl_sign)
        self.assertEqual(
            list(extensions.get_extension_for_class(x509.ExtendedKeyUsage).value),
            [ExtendedKeyUsageOID.CLIENT_AUTH],
        )
        cert.verify_directly_issued_by(self.root_cert)

    def make_intermediate(self):
        key_pem, cert_pem = certificates.generate_intermediate_ca(CLUSTER_ID, 30)
        return (
            x509.load_pem_x509_certificate(cert_pem.encode()),
            serialization.load_pem_private_key(key_pem.encode(), password=None),
        )

    def make_intermediate_issued_leaf(self, uri):
        """A leaf a cluster could sign with its own intermediate CA, naming any identity."""
        intermediate_cert, intermediate_key = self.make_intermediate()
        now = datetime.now(timezone.utc)
        leaf_key = ec.generate_private_key(ec.SECP256R1())
        return (
            x509.CertificateBuilder()
            .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "leaf")]))
            .issuer_name(intermediate_cert.subject)
            .public_key(leaf_key.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(now)
            .not_valid_after(now + timedelta(days=1))
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .add_extension(
                x509.SubjectAlternativeName([x509.UniformResourceIdentifier(uri)]), critical=False
            )
            .sign(intermediate_key, hashes.SHA256())
        )


def sans(cert):
    try:
        return cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    except x509.ExtensionNotFound:
        return []


# ---------------------------------------------------------------------------
# Signing
# ---------------------------------------------------------------------------


class SignCsrTest(RootCaTestCase):
    def test_csr_cannot_choose_identity_ca_or_usages(self):
        csr = make_csr("Oakestra Root CA", GREEDY_EXTENSIONS)

        cert = x509.load_pem_x509_certificate(
            certificates.sign_csr_pem(
                csr, common_name="client-1", alt_names=["client-1"], valid_days=30
            ).encode()
        )

        self.assert_client_leaf(cert)
        self.assertEqual(cert.subject.rfc4514_string(), "CN=client-1")
        self.assertEqual(list(sans(cert)), [x509.DNSName("client-1")])

    def test_uses_the_csr_public_key(self):
        csr = make_csr("client-1")
        cert = x509.load_pem_x509_certificate(
            certificates.sign_csr_pem(csr, "client-1", [], valid_days=30).encode()
        )
        self.assertEqual(cert.public_key(), x509.load_pem_x509_csr(csr.encode()).public_key())

    def test_validity_is_capped_at_ca_expiry(self):
        cert = x509.load_pem_x509_certificate(
            certificates.sign_csr_pem(make_csr("c"), "c", [], valid_days=3650).encode()
        )
        self.assertLessEqual(cert.not_valid_after_utc, self.root_cert.not_valid_after_utc)

    def test_rejects_garbage(self):
        with self.assertRaises(ValueError):
            certificates.sign_csr_pem("not a csr", "c", [], valid_days=30)


class GenerateKeyAndSignedCertTest(RootCaTestCase):
    def test_defaults_to_client_only(self):
        _, cert_pem = certificates.generate_key_and_signed_cert("cluster-a", ["cluster-a"], 30)
        self.assert_client_leaf(x509.load_pem_x509_certificate(cert_pem.encode()))

    def test_spiffe_names_become_uri_sans(self):
        _, cert_pem = certificates.generate_key_and_signed_cert(
            "cluster-a", ["spiffe://oakestra/cluster/abc", "10.0.0.1", "cluster-a"], 30
        )
        san = sans(x509.load_pem_x509_certificate(cert_pem.encode()))
        self.assertEqual(
            san.get_values_for_type(x509.UniformResourceIdentifier),
            ["spiffe://oakestra/cluster/abc"],
        )
        self.assertEqual(
            san.get_values_for_type(x509.IPAddress), [ipaddress.ip_address("10.0.0.1")]
        )
        self.assertEqual(san.get_values_for_type(x509.DNSName), ["cluster-a"])

    def test_server_cert_keeps_both_usages_and_carries_the_root_identity(self):
        # A caller-supplied SPIFFE name is dropped; the root's own is always set.
        certificates.regenerate_server_files("root", ["root", CLUSTER_URI], 30)
        cert = x509.load_pem_x509_certificate(certificates.get_server_cert_path().read_bytes())
        self.assertEqual(
            list(cert.extensions.get_extension_for_class(x509.ExtendedKeyUsage).value),
            [ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH],
        )
        self.assertEqual(certificates.spiffe_id(cert), certificates.ROOT_SPIFFE_ID)
        self.assertEqual(sans(cert).get_values_for_type(x509.DNSName), ["root"])

    def test_reissue_keeps_the_root_identity(self):
        certificates.regenerate_server_files("root", ["root"], 30)
        certificates.reissue_server_files(valid_days=30)
        cert = x509.load_pem_x509_certificate(certificates.get_server_cert_path().read_bytes())
        self.assertEqual(certificates.spiffe_id(cert), certificates.ROOT_SPIFFE_ID)


class CaSubjectTest(RootCaTestCase):
    """The gateway tells root-issued from cluster-issued certs by these markers."""

    def ous(self, cert):
        return [
            a.value for a in cert.subject.get_attributes_for_oid(NameOID.ORGANIZATIONAL_UNIT_NAME)
        ]

    def test_root_ca_is_marked_whatever_its_name(self):
        certificates.regenerate_ca_files(valid_days=30, common_name="OU=oakestra-cluster-ca")
        root_cert, _ = certificates.load_ca_material()
        self.assertEqual(self.ous(root_cert), [certificates.ROOT_CA_MARKER])

    def test_intermediate_name_is_fixed_by_the_root(self):
        intermediate, _ = self.make_intermediate()
        self.assertEqual(self.ous(intermediate), [certificates.CLUSTER_CA_MARKER])
        common_name = intermediate.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value
        self.assertTrue(common_name.startswith(f"Oakestra Cluster CA {CLUSTER_ID} "))
        self.assertEqual(list(sans(intermediate)), [])

    def test_intermediate_needs_a_valid_cluster_id(self):
        with self.assertRaises(ValueError):
            certificates.generate_intermediate_ca("Oakestra Root CA", 30)


class IdentityTest(unittest.TestCase):
    def test_client_names_cannot_escape_their_namespace(self):
        for name in ("../root", "a/b", "", "x" * 65):
            with self.assertRaises(ValueError, msg=name):
                certificates.client_spiffe_id(name)
        self.assertEqual(
            certificates.client_spiffe_id("ops-laptop"), "spiffe://oakestra/client/ops-laptop"
        )

    def test_cluster_ids_must_be_object_ids(self):
        with self.assertRaises(ValueError):
            certificates.cluster_spiffe_id("abc/worker/def")


class IssuedByRootCaTest(RootCaTestCase):
    def test_accepts_root_issued(self):
        _, cert_pem = certificates.generate_key_and_signed_cert("cluster-a", [], 30)
        self.assertTrue(
            certificates.issued_by_root_ca(x509.load_pem_x509_certificate(cert_pem.encode()))
        )

    def test_rejects_intermediate_issued(self):
        # Chains to the root, so the gateway accepts it; the root must not treat it as its own.
        leaf = self.make_intermediate_issued_leaf(CLUSTER_URI)
        self.assertFalse(certificates.issued_by_root_ca(leaf))

    def test_accepts_old_ca_during_rotation(self):
        _, old_cert_pem = certificates.generate_key_and_signed_cert("cluster-a", [], 30)
        old_ca_pem = certificates.get_ca_cert_path().read_text()
        certificates.regenerate_ca_files(valid_days=30, common_name="Oakestra Root CA 2")
        # The dual-CA bundle a rotation with a grace period writes.
        bundle = certificates.get_ca_cert_path().read_text() + "\n" + old_ca_pem
        certificates.get_ca_cert_path().write_text(bundle)

        self.assertTrue(
            certificates.issued_by_root_ca(x509.load_pem_x509_certificate(old_cert_pem.encode()))
        )


# ---------------------------------------------------------------------------
# POST /api/certs/cluster-renew
# ---------------------------------------------------------------------------


class ClusterRenewTest(RootCaTestCase):
    def setUp(self):
        super().setUp()
        self.app = Flask(__name__)
        _, cert_pem = certificates.generate_key_and_signed_cert(CLUSTER_ID, [CLUSTER_URI], 30)
        self.presented_pem = cert_pem

    def renew(self, body, client_cert=None):
        headers = {"X-Client-Cert": quote(client_cert)} if client_cert else {}
        with self.app.test_request_context(
            "/api/certs/cluster-renew", method="POST", json=body, headers=headers
        ):
            return ClusterCertRenewController().post()

    def assert_aborts(self, code, body, client_cert=None):
        with self.assertRaises(HTTPException) as raised:
            self.renew(body, client_cert)
        self.assertEqual(raised.exception.code, code)

    def test_requires_a_client_cert(self):
        self.assert_aborts(401, {"csr": make_csr("cluster-a")})

    def test_rejects_cert_not_signed_by_the_root(self):
        # A cluster forging another cluster's identity with its own intermediate.
        leaf = self.make_intermediate_issued_leaf(f"spiffe://oakestra/cluster/{OTHER_CLUSTER_ID}")
        self.assert_aborts(
            401,
            {"csr": make_csr("cluster-a")},
            leaf.public_bytes(serialization.Encoding.PEM).decode(),
        )

    def test_rejects_root_issued_certs_that_are_not_clusters(self):
        certificates.regenerate_server_files("root", ["root"], 30)
        _, admin_client = certificates.generate_key_and_signed_cert(
            "ops", [certificates.client_spiffe_id("ops")], 30
        )
        for cert_pem in (certificates.get_server_cert_path().read_text(), admin_client):
            self.assert_aborts(403, {"csr": make_csr("cluster-a")}, cert_pem)

    def test_rejects_the_cluster_intermediate_as_client_cert(self):
        # Signed by the root and the cluster holds its key, but it is a CA, not an identity.
        intermediate_cert, _ = self.make_intermediate()
        self.assert_aborts(
            401,
            {"csr": make_csr("cluster-a")},
            intermediate_cert.public_bytes(serialization.Encoding.PEM).decode(),
        )

    def test_keeps_presented_identity_whatever_the_csr_asks(self):
        body = {"csr": make_csr("cluster-b", GREEDY_EXTENSIONS), "valid_days": 100000}

        response = self.renew(body, self.presented_pem)

        cert = x509.load_pem_x509_certificate(response["client_cert"].encode())
        self.assert_client_leaf(cert)
        self.assertEqual(cert.subject.rfc4514_string(), f"CN={CLUSTER_ID}")
        self.assertEqual(list(sans(cert)), [x509.UniformResourceIdentifier(CLUSTER_URI)])
        self.assertLessEqual(
            cert.not_valid_after_utc - cert.not_valid_before_utc, timedelta(days=366)
        )

    def test_rotated_intermediate_is_for_the_presented_cluster(self):
        body = {"csr": make_csr("Oakestra Root CA"), "rotate_intermediate": True}

        response = self.renew(body, self.presented_pem)

        intermediate = x509.load_pem_x509_certificate(response["cluster_ca_cert"].encode())
        common_name = intermediate.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value
        self.assertTrue(common_name.startswith(f"Oakestra Cluster CA {CLUSTER_ID} "))
        constraints = intermediate.extensions.get_extension_for_class(x509.BasicConstraints).value
        self.assertTrue(constraints.ca)
        self.assertEqual(constraints.path_length, 0)


# ---------------------------------------------------------------------------
# POST /api/certs/cluster-bootstrap
# ---------------------------------------------------------------------------


class ClusterBootstrapTest(RootCaTestCase):
    def bootstrap(self, token_doc):
        with mock.patch.object(
            certificates_blueprints, "consume_registration_token", return_value=token_doc
        ):
            with Flask(__name__).test_request_context(
                "/api/certs/cluster-bootstrap",
                method="POST",
                json={"token": "t", "cluster_name": "edge-1"},
            ):
                return certificates_blueprints.ClusterBootstrapController().post()

    def test_issues_material_for_the_tokens_cluster_id(self):
        response = self.bootstrap({"cluster_id": CLUSTER_ID})

        self.assertEqual(response["cluster_id"], CLUSTER_ID)
        client = x509.load_pem_x509_certificate(response["client_cert"].encode())
        self.assert_client_leaf(client)
        self.assertEqual(certificates.cert_cluster_id(client), CLUSTER_ID)
        intermediate = x509.load_pem_x509_certificate(response["cluster_ca_cert"].encode())
        common_name = intermediate.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value
        self.assertTrue(common_name.startswith(f"Oakestra Cluster CA {CLUSTER_ID} "))

    def test_rejects_invalid_tokens(self):
        for token_doc in (None, {}):
            with self.assertRaises(HTTPException) as raised:
                self.bootstrap(token_doc)
            self.assertEqual(raised.exception.code, 401)


# ---------------------------------------------------------------------------
# POST /api/information/<cluster_id>
# ---------------------------------------------------------------------------


class ClusterInformationTest(RootCaTestCase):
    def setUp(self):
        super().setUp()
        patcher = mock.patch.object(clusters_blueprints, "GATEWAY_ENABLED", True)
        patcher.start()
        self.addCleanup(patcher.stop)
        # Stops the handler right after the identity check.
        patcher = mock.patch.object(
            clusters_blueprints.candidate_operations,
            "update_candidate_information",
            return_value=None,
        )
        self.update = patcher.start()
        self.addCleanup(patcher.stop)

    def post(self, cluster_id, client_cert=None):
        headers = {"X-Client-Cert": quote(client_cert)} if client_cert else {}
        with Flask(__name__).test_request_context(
            f"/api/information/{cluster_id}", method="POST", json={"jobs": []}, headers=headers
        ):
            with self.assertRaises(HTTPException) as raised:
                clusters_blueprints.ClusterController().post(clusterid=cluster_id)
        return raised.exception.code

    def cluster_cert(self, cluster_id):
        return certificates.generate_key_and_signed_cert(
            cluster_id, [certificates.cluster_spiffe_id(cluster_id)], 30
        )[1]

    def test_a_cluster_may_update_itself(self):
        # 400 is the patched update failing, i.e. the identity check passed.
        self.assertEqual(self.post(CLUSTER_ID, self.cluster_cert(CLUSTER_ID)), 400)
        self.update.assert_called_once()

    def test_a_cluster_may_not_update_another(self):
        self.assertEqual(self.post(OTHER_CLUSTER_ID, self.cluster_cert(CLUSTER_ID)), 403)
        self.update.assert_not_called()

    def test_requires_a_cluster_cert(self):
        self.assertEqual(self.post(CLUSTER_ID), 401)
        self.update.assert_not_called()


if __name__ == "__main__":
    unittest.main()
