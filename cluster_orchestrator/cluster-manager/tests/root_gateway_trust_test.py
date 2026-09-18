import os
import tempfile
import unittest
from unittest import mock

import bootstrap_certificates
import config


class RootGatewayTrustTestCase(unittest.TestCase):
    def setUp(self):
        handle, self.ca_file = tempfile.mkstemp(suffix=".crt")
        os.write(handle, b"-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n")
        os.close(handle)
        self.addCleanup(os.remove, self.ca_file)

    def configured(self, value):
        return mock.patch.multiple(config, ROOT_GATEWAY_TRUST=value, ROOT_CA_FILE=self.ca_file)


class ParseRootGatewayTrustTest(RootGatewayTrustTestCase):
    def test_system(self):
        trust = config.parse_root_gateway_trust("system", self.ca_file)
        self.assertEqual(trust, config.GatewayTrust(config.TRUST_SYSTEM))

    def test_empty_uses_internal_ca(self):
        trust = config.parse_root_gateway_trust("", self.ca_file)
        self.assertEqual(trust, config.GatewayTrust(config.TRUST_CA_BUNDLE, self.ca_file))

    def test_empty_without_internal_ca(self):
        with self.assertRaisesRegex(ValueError, "ROOT_CA_FILE is not set"):
            config.parse_root_gateway_trust("", None)

    def test_insecure_does_not_fall_back_to_internal_ca(self):
        trust = config.parse_root_gateway_trust("insecure", self.ca_file)
        self.assertEqual(trust, config.GatewayTrust(config.TRUST_INSECURE))

    def test_existing_path(self):
        trust = config.parse_root_gateway_trust(self.ca_file, None)
        self.assertEqual(trust, config.GatewayTrust(config.TRUST_CA_BUNDLE, self.ca_file))

    def test_missing_path(self):
        with self.assertRaisesRegex(ValueError, "does not exist inside the container"):
            config.parse_root_gateway_trust("/home/someone/root-ca.crt", self.ca_file)


class RequestsVerifyTest(RootGatewayTrustTestCase):
    def test_values(self):
        for value, expected in (("system", True), ("insecure", False), ("", self.ca_file)):
            with self.subTest(value=value), self.configured(value):
                self.assertEqual(config.root_gateway_verify(), expected)

    def test_missing_path_raises(self):
        with self.configured("/does/not/exist.crt"), self.assertRaises(ValueError):
            config.root_gateway_verify()


class GrpcRootCertificatesTest(RootGatewayTrustTestCase):
    def test_system_uses_os_trust_store(self):
        with self.configured("system"):
            self.assertIsNone(config.root_gateway_grpc_root_certificates())

    def test_ca_bundle_is_read(self):
        with self.configured(self.ca_file):
            with open(self.ca_file, "rb") as f:
                self.assertEqual(config.root_gateway_grpc_root_certificates(), f.read())

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

    def test_insecure_skips_verification_without_tofu(self):
        verify, tofu = self.resolve("insecure")
        self.assertIs(verify, False)
        tofu.assert_not_called()

    def test_system(self):
        verify, tofu = self.resolve("system")
        self.assertIs(verify, True)
        tofu.assert_not_called()

    def test_empty_fetches_internal_ca_then_uses_it(self):
        verify, tofu = self.resolve("")
        self.assertEqual(verify, self.ca_file)
        tofu.assert_called_once()

    def test_missing_path_exits(self):
        with self.assertRaises(SystemExit):
            self.resolve("/does/not/exist.crt")


if __name__ == "__main__":
    unittest.main()
