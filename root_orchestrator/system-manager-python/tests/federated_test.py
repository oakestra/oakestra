from unittest.mock import MagicMock, patch

import mongomock as mongomock
import pytest
import requests
from ext_requests import mongodb_client, organization_db, user_db
from flask import Flask
from users import federated
from users.auth import user_login
from users.federated import FederatedLoginError, federated_login, internal_login_only

PROVIDER = "users.federated.requests.post"


@pytest.fixture
def enabled(monkeypatch):
    monkeypatch.setenv("FEDERATED_LOGIN_AUTHORIZE_URL", "https://idp.example/authorize")
    monkeypatch.setenv("FEDERATED_LOGIN_CLUSTER_ID", "cluster-1")
    monkeypatch.setenv("FEDERATED_LOGIN_CLUSTER_SECRET", "s3cret")
    monkeypatch.setenv("INTERNAL_LOGIN", "false")


@pytest.fixture
def tokens():
    with patch("users.federated.securityUtils") as su:
        su.create_jwt_auth_access_token.return_value = "access"
        su.create_jwt_auth_refresh_token.return_value = "refresh"
        yield su


def provider_answer(status=200, body=None):
    r = MagicMock()
    r.status_code = status
    r.json.return_value = body
    return r


def mockdb():
    mongodb_client.mongo_users = mongomock.MongoClient().db.users
    mongodb_client.mongo_organization = mongomock.MongoClient().db.organization
    user_db.create_admin()


def root_id():
    return str(organization_db.mongo_get_organization_by_name("root")["_id"])


def test_defaults_keep_internal_login_only(monkeypatch):
    monkeypatch.delenv("FEDERATED_LOGIN_AUTHORIZE_URL", raising=False)
    monkeypatch.delenv("INTERNAL_LOGIN", raising=False)
    assert federated.login_methods() == {"internal": True, "federated": False}


def test_disabled_federated_login_is_not_found(monkeypatch):
    monkeypatch.delenv("FEDERATED_LOGIN_AUTHORIZE_URL", raising=False)
    with pytest.raises(FederatedLoginError) as e:
        federated_login("token")
    assert e.value.status == 404


def test_internal_login_only_blocks_when_disabled(monkeypatch):
    view = internal_login_only(lambda: "ok")
    monkeypatch.setenv("INTERNAL_LOGIN", "false")
    body, status = view()
    assert status == 403
    monkeypatch.setenv("INTERNAL_LOGIN", "true")
    assert view() == "ok"


@mongomock.patch(servers=(("localhost", 10007),))
def test_first_login_provisions_user_with_roles(enabled, tokens):
    mockdb()
    answer = {
        "username": "ada@example.com",
        "email": "ada@example.com",
        "roles": ["Admin", "Bogus"],
    }
    with patch(PROVIDER, return_value=provider_answer(body=answer)) as post:
        resp = federated_login("provider-token")

    assert resp == {"token": "access", "refresh_token": "refresh"}
    _, kwargs = post.call_args
    assert kwargs["headers"]["Authorization"] == "Bearer provider-token"
    assert kwargs["json"] == {"cluster_id": "cluster-1", "cluster_secret": "s3cret"}

    user = user_db.mongo_get_user_by_name("ada@example.com", root_id())
    assert user["federated"] is True
    assert user["password"] is None
    assert user["roles"] == ["Admin"]  # unknown roles are dropped
    claims = tokens.create_jwt_auth_access_token.call_args.kwargs["additional_claims"]
    assert claims == {
        "user": "ada@example.com",
        "roles": ["Admin"],
        "organization": root_id(),
        "federated": True,
    }
    assert tokens.create_jwt_auth_refresh_token.call_args.kwargs["expires_delta"].seconds == 3600


@mongomock.patch(servers=(("localhost", 10007),))
def test_roles_are_synced_on_every_login(enabled, tokens):
    mockdb()
    with patch(
        PROVIDER, return_value=provider_answer(body={"username": "bob", "roles": ["Admin"]})
    ):
        federated_login("t")
    with patch(
        PROVIDER,
        return_value=provider_answer(body={"username": "bob", "roles": ["Application_Provider"]}),
    ):
        federated_login("t")

    assert len(user_db.mongo_get_user()) == 2  # Admin + bob, no duplicate
    assert user_db.mongo_get_user_by_name("bob", root_id())["roles"] == ["Application_Provider"]


@mongomock.patch(servers=(("localhost", 10007),))
def test_cannot_take_over_local_user(enabled, tokens):
    mockdb()
    with patch(
        PROVIDER, return_value=provider_answer(body={"username": "Admin", "roles": ["Admin"]})
    ), pytest.raises(FederatedLoginError) as e:
        federated_login("t")
    assert e.value.status == 409
    tokens.create_jwt_auth_access_token.assert_not_called()


@pytest.mark.parametrize(
    "status,body,expected",
    [
        (401, None, 401),
        (403, None, 403),
        (404, None, 403),
        (500, None, 502),
        (200, {"username": "x", "roles": []}, 403),
        (200, {"username": "x", "roles": ["NotARole"]}, 403),
        (200, {"username": "../evil", "roles": ["Admin"]}, 502),
        (200, ["not", "an", "object"], 502),
    ],
)
def test_provider_refusals(enabled, tokens, status, body, expected):
    with patch(PROVIDER, return_value=provider_answer(status, body)):
        with pytest.raises(FederatedLoginError) as e:
            federated_login("t")
    assert e.value.status == expected
    tokens.create_jwt_auth_access_token.assert_not_called()


def test_provider_unreachable(enabled, tokens):
    with patch(PROVIDER, side_effect=requests.ConnectionError("down")):
        with pytest.raises(FederatedLoginError) as e:
            federated_login("t")
    assert e.value.status == 502


def test_missing_token(enabled):
    with pytest.raises(FederatedLoginError) as e:
        federated_login("  ")
    assert e.value.status == 400


@mongomock.patch(servers=(("localhost", 10007),))
def test_password_login_rejects_federated_user(enabled, tokens):
    mockdb()
    with patch(
        PROVIDER, return_value=provider_answer(body={"username": "eve", "roles": ["Admin"]})
    ):
        federated_login("t")
    with patch("users.auth.securityUtils") as su:
        assert user_login({"username": "eve", "password": "anything"}) == {}
        su.create_jwt_auth_access_token.assert_not_called()


def test_methods_endpoint(enabled):
    from blueprints.federated_blueprints import LoginMethodsController

    with Flask(__name__).test_request_context():
        assert LoginMethodsController().get() == {"internal": False, "federated": True}
