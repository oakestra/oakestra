"""Fixtures for the jwt-generator test suite.

jwt_generator.py reads JWT_PRIVATE_KEY / JWT_PUBLIC_KEY from the environment
at import time, so each scenario (auto-generated keys vs. keys supplied via
env vars) needs its own fresh import of the module.
"""

import importlib
import sys

import pytest
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa

MODULE_NAME = "jwt_generator"


@pytest.fixture(scope="session")
def rsa_keypair():
    """A fixed PEM keypair used to exercise the JWT_PRIVATE_KEY/JWT_PUBLIC_KEY env path."""
    private_key_obj = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    public_key_obj = private_key_obj.public_key()

    private_pem = private_key_obj.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")
    public_pem = public_key_obj.public_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PublicFormat.SubjectPublicKeyInfo,
    ).decode("utf-8")

    return private_pem, public_pem


@pytest.fixture
def fresh_module(monkeypatch):
    """Factory fixture: import a fresh jwt_generator module under given env vars.

    Returns a callable(private_key=None, public_key=None) -> module. Each call
    pops the module from sys.modules first so module-level key generation
    runs again, then re-imports it under the requested env.
    """

    def _make(private_key=None, public_key=None):
        if private_key is None:
            monkeypatch.delenv("JWT_PRIVATE_KEY", raising=False)
        else:
            monkeypatch.setenv("JWT_PRIVATE_KEY", private_key)
        if public_key is None:
            monkeypatch.delenv("JWT_PUBLIC_KEY", raising=False)
        else:
            monkeypatch.setenv("JWT_PUBLIC_KEY", public_key)

        sys.modules.pop(MODULE_NAME, None)
        return importlib.import_module(MODULE_NAME)

    yield _make

    sys.modules.pop(MODULE_NAME, None)


@pytest.fixture
def app_env_keys(fresh_module, rsa_keypair):
    """jwt_generator module using a known keypair supplied via env vars."""
    private_pem, public_pem = rsa_keypair
    return fresh_module(private_key=private_pem, public_key=public_pem)


@pytest.fixture
def client(app_env_keys):
    """Flask test client for tests that don't care where the keypair came from. Uses the
    session keypair because generating a 2048-bit key on every import dominates runtime."""
    return app_env_keys.app.test_client()
