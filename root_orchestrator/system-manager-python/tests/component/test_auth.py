"""Component tests for login/register/refresh and the JWT/role enforcement layers -
blueprints/authentication_blueprints.py, users/auth.py, roles/securityUtils.py.
"""

import datetime
import functools

import jwt as pyjwt
import pytest

from tests.component.conftest import (
    ADMIN_PASSWORD,
    ADMIN_USERNAME,
    JWT_PUBLIC_KEY,
    bearer,
    pem_keypair,
)

pytestmark = pytest.mark.component

ROOT_ROLES = {"Admin", "Organization_Admin", "Application_Provider", "Infrastructure_Provider"}


def _decode(token):
    return pyjwt.decode(token, JWT_PUBLIC_KEY, algorithms=["RS256"])


@functools.cache
def _untrusted_private_key():
    # 2048-bit key generation is slow; one untrusted key is enough for every test.
    return pem_keypair()[0]


def _wrong_signature_token(**claims):
    """A structurally valid access token signed by a key the system manager does not
    trust, for exercising the "bad signature" branch of token verification."""
    pem = _untrusted_private_key()
    payload = {
        "sub": "Admin",
        "type": "access",
        "fresh": False,
        "iat": 0,
        "jti": "wrong-key-token",
        "exp": 9999999999,
        "user": "Admin",
        "roles": ["Admin"],
        "organization": "000000000000000000000000",
        **claims,
    }
    return pyjwt.encode(payload, pem, algorithm="RS256")


def _register(client, headers, **overrides):
    body = {
        "name": "newuser",
        "password": "pw123",
        "email": "newuser@example.com",
        "created_at": "01/01/2026 10:00",
        "roles": ["Application_Provider"],
        **overrides,
    }
    return client.post("/api/auth/register", json=body, headers=headers)


# ---------------------------------------------------------------------------- login


def test_login_default_admin_returns_token_and_refresh_token_with_expected_claims(client):
    response = client.post(
        "/api/auth/login", json={"username": ADMIN_USERNAME, "password": ADMIN_PASSWORD}
    )

    assert response.status_code == 200
    body = response.get_json()
    assert "token" in body
    assert "refresh_token" in body

    claims = _decode(body["token"])
    assert claims["user"] == ADMIN_USERNAME
    assert set(claims["roles"]) == ROOT_ROLES
    assert claims["type"] == "access"

    refresh_claims = _decode(body["refresh_token"])
    assert refresh_claims["user"] == ADMIN_USERNAME
    assert refresh_claims["type"] == "refresh"


def test_login_wrong_password_rejected(client):
    response = client.post(
        "/api/auth/login", json={"username": ADMIN_USERNAME, "password": "wrong"}
    )

    assert response.status_code == 401


def test_login_unknown_user_rejected(client):
    response = client.post(
        "/api/auth/login", json={"username": "nobody-registered", "password": "whatever"}
    )

    assert response.status_code == 401


def test_login_missing_body_rejected(client):
    # No json= at all, so request.get_json() sees a non-JSON content type and returns None.
    response = client.post("/api/auth/login")

    assert response.status_code == 403


def test_login_unknown_organization_name_not_found(client):
    response = client.post(
        "/api/auth/login",
        json={
            "username": ADMIN_USERNAME,
            "password": ADMIN_PASSWORD,
            "organization_name": "does-not-exist",
        },
    )

    assert response.status_code == 404


def test_login_with_organization_name_uses_that_organizations_roles(
    client, admin_headers, create_user, mongo
):
    create_user("orguser", roles=("Application_Provider",))
    user_id = str(mongo.users.find_one({"name": "orguser"})["_id"])
    org = client.post(
        "/api/organization/",
        json={
            "name": "other-org",
            "member": [{"user_id": user_id, "roles": ["Organization_Admin"]}],
        },
        headers=admin_headers,
    ).get_json()

    response = client.post(
        "/api/auth/login",
        json={"username": "orguser", "password": "s3cret-pw", "organization_name": "other-org"},
    )

    assert response.status_code == 200
    claims = _decode(response.get_json()["token"])
    assert claims["organization"] == org["_id"]
    assert claims["roles"] == ["Organization_Admin"]


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: authentication_blueprints.py login_schema documents the login field as "
        "'organization', but users/auth.py:59 reads content['organization_name'], so a "
        "client following the documented schema ends up logged into root instead of "
        "the organization it asked for"
    ),
)
def test_login_with_documented_organization_field_logs_into_requested_org(
    client, admin_headers, create_user, mongo
):
    create_user("orguser2", roles=("Application_Provider",))
    user_id = str(mongo.users.find_one({"name": "orguser2"})["_id"])
    org = client.post(
        "/api/organization/",
        json={
            "name": "other-org-2",
            "member": [{"user_id": user_id, "roles": ["Organization_Admin"]}],
        },
        headers=admin_headers,
    ).get_json()

    response = client.post(
        "/api/auth/login",
        json={"username": "orguser2", "password": "s3cret-pw", "organization": "other-org-2"},
    )

    assert response.status_code == 200
    claims = _decode(response.get_json()["token"])
    assert claims["organization"] == org["_id"]
    assert claims["roles"] == ["Organization_Admin"]


# ---------------------------------------------------------------------------- register


def test_register_returns_201_and_new_user_can_login(client, admin_headers, login):
    response = _register(client, admin_headers, name="freshuser", password="pw123")

    assert response.status_code == 201
    assert "token" in login("freshuser", "pw123")


def test_register_duplicate_username_conflict(client, admin_headers):
    first = _register(client, admin_headers, name="dupuser")
    second = _register(client, admin_headers, name="dupuser")

    assert first.status_code == 201
    assert second.status_code == 409


# ---------------------------------------------------------------------------- refresh


def test_refresh_with_refresh_token_returns_new_working_access_token(client, login):
    tokens = login()

    response = client.post("/api/auth/refresh", headers=bearer(tokens["refresh_token"]))

    assert response.status_code == 200
    new_token = response.get_json()["token"]
    probe = client.get("/api/organization/", headers=bearer(new_token))
    assert probe.status_code == 200


def test_refresh_with_access_token_rejected(client, login):
    tokens = login()

    response = client.post("/api/auth/refresh", headers=bearer(tokens["token"]))

    assert response.status_code == 422


# ---------------------------------------------------------------------------- token rejection


PROTECTED_ROUTES = [
    ("GET", "/api/organization/", "jwt_required"),
    ("GET", "/api/application/dummy-app-id", "jwt_required"),
    ("GET", "/api/user/Admin", "jwt_auth_required"),
    ("GET", "/api/permission/Admin", "jwt_auth_required"),
]

# roles.securityUtils.jwt_auth_required() wraps verify_jwt_in_request() in a bare
# except and always answers 401, unlike flask_jwt_extended's own jwt_required(),
# which reports malformed/bad-signature/wrong-token-type tokens as 422.
EXPECTED_STATUS = {
    "jwt_required": {"none": 401, "expired": 401, "wrong_signature": 422, "malformed": 422},
    "jwt_auth_required": {"none": 401, "expired": 401, "wrong_signature": 401, "malformed": 401},
}


def _headers_for(scenario, mint_token):
    if scenario == "none":
        return {}
    if scenario == "expired":
        return bearer(mint_token(expires_delta=datetime.timedelta(seconds=-10)))
    if scenario == "wrong_signature":
        return bearer(_wrong_signature_token())
    return bearer("not-a-jwt")


@pytest.mark.parametrize(
    "method,path,kind",
    PROTECTED_ROUTES,
    ids=[f"{m}-{p}" for m, p, _ in PROTECTED_ROUTES],
)
@pytest.mark.parametrize("scenario", ["none", "expired", "wrong_signature", "malformed"])
def test_protected_route_rejects_bad_token(client, mint_token, scenario, method, path, kind):
    headers = _headers_for(scenario, mint_token)

    response = client.open(path, method=method, headers=headers)

    assert response.status_code == EXPECTED_STATUS[kind][scenario]


def test_file_access_token_rejected_on_jwt_auth_required_route(client, mint_token):
    token = mint_token(file_access_token=True)

    response = client.get("/api/user/Admin", headers=bearer(token))

    assert response.status_code == 401
    assert "access token" in response.get_json()["message"].lower()


# ---------------------------------------------------------------------------- role matrix


ADMIN_ONLY_ROUTES = [
    # /api/applications/ and /api/services/ are tested in their own files.
    ("GET", "/api/users/"),
    ("GET", "/api/organization/"),
]


@pytest.mark.parametrize("method,path", ADMIN_ONLY_ROUTES, ids=[p for _, p in ADMIN_ONLY_ROUTES])
def test_admin_only_route_rejects_non_admin(client, create_user, method, path):
    headers = create_user("non_admin_user")

    response = client.open(path, method=method, headers=headers)

    assert response.status_code == 403


@pytest.mark.parametrize("method,path", ADMIN_ONLY_ROUTES, ids=[p for _, p in ADMIN_ONLY_ROUTES])
def test_admin_only_route_allows_admin(client, admin_headers, method, path):
    response = client.open(path, method=method, headers=admin_headers)

    assert response.status_code == 200


def test_register_route_rejects_non_admin_and_allows_admin(client, admin_headers, create_user):
    non_admin = create_user("non_admin_registrar")

    forbidden = _register(client, non_admin, name="whoever")
    allowed = _register(client, admin_headers, name="whoever")

    assert forbidden.status_code == 403
    assert allowed.status_code == 201


# ---------------------------------------------------------------------------- identity_is_username


def test_user_cannot_read_others_profile(client, create_user):
    alice_headers = create_user("alice")
    create_user("bob")

    other = client.get("/api/user/bob", headers=alice_headers)

    assert other.status_code == 403


def test_user_can_read_own_permissions_but_not_others(client, create_user):
    alice_headers = create_user("alice")
    create_user("bob")

    own = client.get("/api/permission/alice", headers=alice_headers)
    other = client.get("/api/permission/bob", headers=alice_headers)

    assert own.status_code == 200
    assert other.status_code == 403
