"""Component tests for user CRUD, password flows and organization management -
blueprints/users_blueprints.py, blueprints/organization_blueprints.py, the password
flows in users/auth.py, and ext_requests/user_db.py.
"""

import hashlib

import pytest
from bson import ObjectId

pytestmark = pytest.mark.component


@pytest.fixture
def fixed_reset_token(monkeypatch):
    """The raw reset token comes from secrets.token_urlsafe() and never leaves the server:
    the mail carries its hash, and sending is broken anyway (see the xfails below). Fixing
    its value lets a test drive the reset flow with the token a working mail would carry."""
    import secrets as secrets_module

    token = "fixed-reset-token-for-tests"
    monkeypatch.setattr(secrets_module, "token_urlsafe", lambda *a, **k: token)
    return token


# ---------------------------------------------------------------------------- user CRUD


def test_get_own_user_returns_profile(client, create_user):
    headers = create_user("alice", roles=("Application_Provider",))

    response = client.get("/api/user/alice", headers=headers)

    assert response.status_code == 200
    body = response.get_json()
    assert body["name"] == "alice"
    assert body["roles"] == ["Application_Provider"]


def test_admin_lists_all_users(client, admin_headers, create_user):
    create_user("alice")
    create_user("bob")

    response = client.get("/api/users/", headers=admin_headers)

    assert response.status_code == 200
    names = {u["name"] for u in response.get_json()}
    assert {"Admin", "alice", "bob"} <= names


def test_admin_lists_users_by_organization(client, admin_headers, create_user, root_org_id):
    create_user("alice")

    response = client.get(f"/api/users/{root_org_id}", headers=admin_headers)

    assert response.status_code == 200
    names = {u["name"] for u in response.get_json()}
    assert "alice" in names


def test_admin_updates_user_roles(client, admin_headers, create_user):
    alice_headers = create_user("alice", roles=("Application_Provider",))

    response = client.put(
        "/api/user/alice",
        json={"roles": ["Organization_Admin"], "email": "alice2@example.com"},
        headers=admin_headers,
    )

    assert response.status_code == 200
    permissions = client.get("/api/permission/alice", headers=alice_headers)
    assert permissions.get_json()["roles"] == ["Organization_Admin"]


def test_admin_deletes_user(client, admin_headers, create_user):
    create_user("alice")

    response = client.delete("/api/user/alice", headers=admin_headers)

    assert response.status_code == 200
    remaining = client.get("/api/users/", headers=admin_headers)
    assert "alice" not in {u["name"] for u in remaining.get_json()}


def test_deleted_user_cannot_login(client, admin_headers, create_user):
    create_user("alice", password="alicepw")

    client.delete("/api/user/alice", headers=admin_headers)
    response = client.post("/api/auth/login", json={"username": "alice", "password": "alicepw"})

    assert response.status_code == 401


# ---------------------------------------------------------------------------- change password


def test_change_password_old_stops_working_new_works(client, create_user):
    headers = create_user("alice", password="oldpw")

    response = client.post(
        "/api/user/alice",
        json={"oldPassword": "oldpw", "newPassword": "newpw"},
        headers=headers,
    )

    assert response.status_code == 200
    old_login = client.post("/api/auth/login", json={"username": "alice", "password": "oldpw"})
    new_login = client.post("/api/auth/login", json={"username": "alice", "password": "newpw"})
    assert old_login.status_code == 401
    assert new_login.status_code == 200


def test_change_password_wrong_old_password_rejected(client, create_user):
    headers = create_user("alice", password="oldpw")

    response = client.post(
        "/api/user/alice",
        json={"oldPassword": "not-the-old-one", "newPassword": "newpw"},
        headers=headers,
    )

    assert response.status_code == 400


# ---------------------------------------------------------------------------- password reset


def test_password_reset_request_stores_token_for_known_user(client, create_user, mongo):
    create_user("alice")

    response = client.post("/api/user/", json={"username": "alice", "domain": "example.com"})

    assert response.status_code == 200
    stored = mongo.users.find_one({"token_hash": {"$exists": True}})
    assert stored is not None


def test_password_reset_request_does_not_mail_unknown_user(client, smtp):
    client.post("/api/user/", json={"username": "no-such-user", "domain": "example.com"})

    # Don't check send_message: because of the expiry_delta bug below it isn't reached
    # for a known user either. Whether SMTP_SSL was opened at all is what tells the two
    # cases apart.
    assert not smtp.called


def test_password_reset_request_attempts_mail_for_known_user(client, create_user, smtp):
    create_user("alice")

    response = client.post("/api/user/", json={"username": "alice", "domain": "example.com"})

    assert response.status_code == 200
    assert smtp.called


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: users/auth.py user_create_password_reset_request() passes an absolute "
        "datetime as ResetPasswordMailFactory's 'expiry_delta' payload field, but "
        "mail/mail.py ResetPasswordMailFactory.create_message() treats it as a "
        "timedelta (.days/.seconds); the resulting AttributeError is swallowed by "
        "MailFactory.send_mail()'s bare except, so the reset email is never sent"
    ),
)
def test_password_reset_request_sends_email(client, create_user, smtp):
    create_user("alice")

    response = client.post("/api/user/", json={"username": "alice", "domain": "example.com"})

    assert response.status_code == 200
    assert smtp.return_value.send_message.called


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: blueprints/users_blueprints.py UserResetPasswordController wraps the "
        "(body, status) tuple returned by users/auth.py in jsonify() instead of "
        "returning (jsonify(body), status), so the real status code (404 for an "
        "unknown user here) never reaches the HTTP response - the endpoint answers "
        "200 no matter what users/auth.py decided"
    ),
)
def test_password_reset_request_for_unknown_user_returns_404(client):
    response = client.post("/api/user/", json={"username": "no-such-user", "domain": "example.com"})

    assert response.status_code == 404


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: users/auth.py user_create_password_reset_request() builds the mailed "
        "reset link from reset_token_hash (the value already hashed for storage) "
        "instead of the raw reset_token, so the value actually printed in the email a "
        "user receives can never satisfy user_change_password_with_reset_request()'s "
        "hash-and-compare check"
    ),
)
def test_password_reset_mailed_link_value_can_be_used_to_reset(
    client, create_user, fixed_reset_token
):
    create_user("alice", password="oldpw")
    client.post("/api/user/", json={"username": "alice", "domain": "example.com"})
    # Same hashing users/auth.py applies before putting the token into the mail.
    mailed_value = hashlib.pbkdf2_hmac(
        "sha256", fixed_reset_token.encode("ascii"), b"", 100000
    ).hex()

    client.put("/api/user/", json={"token": mailed_value, "password": "newpw"})

    # The PUT always answers 200 (jsonify(tuple) bug above), so check whether the
    # password actually changed.
    login_with_new_password = client.post(
        "/api/auth/login", json={"username": "alice", "password": "newpw"}
    )
    assert login_with_new_password.status_code == 200


def test_password_reset_apply_rejects_wrong_token(client, create_user, fixed_reset_token):
    create_user("alice", password="oldpw")
    client.post("/api/user/", json={"username": "alice", "domain": "example.com"})

    # Always 200, see above.
    client.put("/api/user/", json={"token": "not-the-real-token", "password": "whatever"})

    still_old = client.post("/api/auth/login", json={"username": "alice", "password": "oldpw"})
    rejected_new = client.post(
        "/api/auth/login", json={"username": "alice", "password": "whatever"}
    )
    assert still_old.status_code == 200
    assert rejected_new.status_code == 401


def test_password_reset_apply_with_raw_token_works_and_cannot_be_reused(
    client, create_user, fixed_reset_token
):
    create_user("alice", password="oldpw")
    client.post("/api/user/", json={"username": "alice", "domain": "example.com"})

    first = client.put("/api/user/", json={"token": fixed_reset_token, "password": "newpw"})

    assert first.status_code == 200
    old_login = client.post("/api/auth/login", json={"username": "alice", "password": "oldpw"})
    new_login = client.post("/api/auth/login", json={"username": "alice", "password": "newpw"})
    assert old_login.status_code == 401
    assert new_login.status_code == 200

    # Always 200 as well, so a rejected reuse only shows in the login below.
    client.put("/api/user/", json={"token": fixed_reset_token, "password": "yet-another"})

    unaffected_login = client.post(
        "/api/auth/login", json={"username": "alice", "password": "newpw"}
    )
    assert unaffected_login.status_code == 200


# ---------------------------------------------------------------------------- organizations


def test_admin_creates_lists_updates_deletes_organization(client, admin_headers, mongo):
    created = client.post(
        "/api/organization/", json={"name": "acme", "member": []}, headers=admin_headers
    )
    assert created.status_code == 200
    org_id = created.get_json()["_id"]

    listing = client.get("/api/organization/", headers=admin_headers)
    assert listing.status_code == 200
    assert any(o["name"] == "acme" for o in listing.get_json())

    updated = client.put(
        f"/api/organization/{org_id}",
        json={"name": "acme-renamed", "member": []},
        headers=admin_headers,
    )
    assert updated.status_code == 200
    assert mongo.organizations.find_one({"_id": ObjectId(org_id)})["name"] == "acme-renamed"

    deleted = client.delete(f"/api/organization/{org_id}", headers=admin_headers)
    assert deleted.status_code == 200
    assert mongo.organizations.find_one({"_id": ObjectId(org_id)}) is None


def test_organization_create_rejects_non_admin(client, create_user):
    headers = create_user("plain_user")

    response = client.post("/api/organization/", json={"name": "x", "member": []}, headers=headers)

    assert response.status_code == 403


def test_organization_update_and_delete_reject_non_admin(client, admin_headers, create_user):
    org = client.post(
        "/api/organization/", json={"name": "acme", "member": []}, headers=admin_headers
    ).get_json()
    non_admin = create_user("plain_user")

    update = client.put(
        f"/api/organization/{org['_id']}",
        json={"name": "y", "member": []},
        headers=non_admin,
    )
    delete = client.delete(f"/api/organization/{org['_id']}", headers=non_admin)

    assert update.status_code == 403
    assert delete.status_code == 403
