"""OIDC login, renewal, sign-out, management lock and emergency login against a fake provider.

The fake provider signs real ID tokens (RS256) and serves discovery/JWKS/userinfo through a patched
requests.get; the token and device endpoints are served through a patched OAuth2Session.post.
Oakestra tokens are minted locally with flask_jwt_extended instead of the jwt_generator service.
"""

import json
import time
from datetime import timedelta
from unittest.mock import patch
from urllib.parse import parse_qs, urlsplit

import mongomock as mongomock
import pytest
import requests
from authlib.integrations.requests_client import OAuth2Session
from authlib.jose import JsonWebKey, jwt
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from ext_requests import mongodb_client, organization_db, user_db
from flask import Flask
from flask_jwt_extended import JWTManager, create_access_token, create_refresh_token
from flask_smorest import Api
from users import emergency, federated, oidc, sessions, signout_feed
from utils.json_encoder import MongoJSONEncoder

ISSUER = "https://idp.example/realms/lab"
CLIENT_ID = "cluster-1"
CALLBACK = "http://192.168.1.10:10000/api/auth/oidc/callback"
DASHBOARD = "http://192.168.1.10/"
MOCK_SERVERS = (("localhost", 10007),)


# ............ fake provider ............ #


class FakeResponse:
    def __init__(self, status=200, body=None):
        self.status_code = status
        self._body = body

    def json(self):
        if isinstance(self._body, Exception):
            raise self._body
        return self._body

    def raise_for_status(self):
        if self.status_code >= 400:
            raise requests.HTTPError(f"HTTP {self.status_code}")


class FakeProvider:
    def __init__(self):
        self.key = JsonWebKey.generate_key("RSA", 2048, is_private=True, options={"kid": "k1"})
        self.discovery = {
            "issuer": ISSUER,
            "authorization_endpoint": ISSUER + "/auth",
            "token_endpoint": ISSUER + "/token",
            "jwks_uri": ISSUER + "/jwks",
            "userinfo_endpoint": ISSUER + "/userinfo",
            "device_authorization_endpoint": ISSUER + "/device",
            "id_token_signing_alg_values_supported": ["RS256"],
        }
        self.sub = "user-1"
        self.claims = {"preferred_username": "ada@example.com", "email": "ada@example.com"}
        self.roles = ["Admin"]
        self.nonce = None
        self.userinfo = None
        self.token_error = None  # (status, body) or an exception to raise
        self.include_id_token = True
        self.refresh_counter = 0
        self.token_requests = []

    def id_token(self, **overrides):
        now = int(time.time())
        payload = {
            "iss": ISSUER,
            "sub": self.sub,
            "aud": CLIENT_ID,
            "iat": now,
            "exp": now + 300,
            "oakestra_roles": self.roles,
            **self.claims,
        }
        if self.nonce:
            payload["nonce"] = self.nonce
        payload.update(overrides)
        header = {"alg": "RS256", "kid": self.key.kid}
        return jwt.encode(header, payload, self.key).decode()

    def get(self, url, **kwargs):
        if url == ISSUER + "/.well-known/openid-configuration":
            return FakeResponse(body=self.discovery)
        if url == ISSUER + "/jwks":
            return FakeResponse(body={"keys": [self.key.as_dict()]})
        if url == ISSUER + "/userinfo":
            if self.userinfo is None:
                return FakeResponse(body={"sub": self.sub})
            return FakeResponse(body=self.userinfo)
        return FakeResponse(404, {})

    def post(self, session, url, data=None, **kwargs):
        self.token_requests.append((url, dict(data or {}), kwargs))
        if url == ISSUER + "/device":
            return FakeResponse(
                body={
                    "device_code": "dev-1",
                    "user_code": "ABCD-EFGH",
                    "verification_uri": ISSUER + "/device-ui",
                    "verification_uri_complete": ISSUER + "/device-ui?code=ABCD-EFGH",
                    "expires_in": 600,
                    "interval": 5,
                }
            )
        if isinstance(self.token_error, Exception):
            raise self.token_error
        if self.token_error is not None:
            return FakeResponse(*self.token_error)
        self.refresh_counter += 1
        body = {
            "access_token": "at-%d" % self.refresh_counter,
            "token_type": "Bearer",
            "expires_in": 300,
            "refresh_token": "rt-%d" % self.refresh_counter,
        }
        if self.include_id_token:
            body["id_token"] = self.id_token()
        return FakeResponse(body=body)


@pytest.fixture
def provider(monkeypatch):
    p = FakeProvider()
    monkeypatch.setenv("OIDC_ISSUER", ISSUER)
    monkeypatch.setenv("OIDC_CLIENT_ID", CLIENT_ID)
    monkeypatch.setenv("OIDC_CLIENT_SECRET", "s3cret")
    monkeypatch.setenv("OIDC_REDIRECT_URIS", CALLBACK + ",http://lab.tailnet.ts.net:10000/cb")
    monkeypatch.setenv("OIDC_ROLES_CLAIM", "oakestra_roles")
    monkeypatch.setenv("OIDC_PROVIDER_NAME", "Lab IdP")
    monkeypatch.setenv("INTERNAL_LOGIN", "false")
    for name in ("OIDC_ROLE_MAP", "OIDC_USERNAME_CLAIM", "OIDC_ALLOWED_RETURN_HOSTS"):
        monkeypatch.delenv(name, raising=False)
    oidc.reset_cache()
    with (
        patch("users.oidc.requests.get", side_effect=p.get),
        patch.object(
            OAuth2Session, "post", autospec=True, side_effect=lambda *a, **k: p.post(*a, **k)
        ),
    ):
        yield p
    oidc.reset_cache()


# ............ Mongo and Oakestra tokens ............ #


def mockdb():
    client = mongomock.MongoClient()
    mongodb_client.mongo_users = client.db.user
    mongodb_client.mongo_organization = client.db.organization
    user_db.create_admin()
    sessions._invalidate_revocation_cache()


def root_id():
    return str(organization_db.mongo_get_organization_by_name("root")["_id"])


def roles_of(username):
    return user_db.mongo_get_user_by_name(username, root_id()).get("roles")


def _pem_keys():
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    private = key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.TraditionalOpenSSL,
        serialization.NoEncryption(),
    ).decode()
    public = (
        key.public_key()
        .public_bytes(serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo)
        .decode()
    )
    return private, public


PRIVATE_PEM, PUBLIC_PEM = _pem_keys()


def local_access_token(identity, additional_claims, expires_delta=None):
    return create_access_token(
        identity=identity, additional_claims=additional_claims, expires_delta=expires_delta
    )


def local_refresh_token(identity, additional_claims, expires_delta=None):
    return create_refresh_token(
        identity=identity, additional_claims=additional_claims, expires_delta=expires_delta
    )


@pytest.fixture
def app():
    from blueprints.authentication_blueprints import loginbp
    from blueprints.federated_blueprints import federatedbp, signoutbp
    from blueprints.organization_blueprints import organizationblp
    from blueprints.users_blueprints import userbp, usersbp

    flask_app = Flask(__name__)
    flask_app.json_encoder = MongoJSONEncoder
    flask_app.config.update(
        API_TITLE="test",
        API_VERSION="v1",
        OPENAPI_VERSION="3.0.2",
        JWT_ALGORITHM="RS256",
        JWT_PRIVATE_KEY=PRIVATE_PEM,
        JWT_PUBLIC_KEY=PUBLIC_PEM,
        TESTING=True,
    )
    manager = JWTManager(flask_app)
    manager.token_in_blocklist_loader(sessions.token_revoked)
    api = Api(flask_app)
    for bp in (loginbp, federatedbp, signoutbp, organizationblp, userbp, usersbp):
        api.register_blueprint(bp)
    with (
        patch("roles.securityUtils.create_access_token", side_effect=local_access_token),
        patch("roles.securityUtils.create_refresh_token", side_effect=local_refresh_token),
    ):
        with flask_app.app_context():
            yield flask_app


def decode(token):
    from flask_jwt_extended import decode_token

    return decode_token(token)


def bearer(token):
    return {"Authorization": "Bearer " + token}


# ............ helpers driving the browser flow ............ #


def start_login(client, return_to=DASHBOARD, host="192.168.1.10:10000"):
    r = client.get(
        "/api/auth/oidc/login", query_string={"return_to": return_to}, headers={"Host": host}
    )
    return r


def browser_login(client, provider, **id_token_overrides):
    r = start_login(client)
    assert r.status_code == 302, r.data
    query = parse_qs(urlsplit(r.headers["Location"]).query)
    provider.nonce = query["nonce"][0]
    if id_token_overrides:
        provider.claims.update(id_token_overrides)
    return client.get(
        "/api/auth/oidc/callback", query_string={"code": "c1", "state": query["state"][0]}
    ), query


def fragment(response):
    return dict(parse_qs(urlsplit(response.headers["Location"]).fragment))


def login_tokens(client, provider):
    r, _ = browser_login(client, provider)
    code = fragment(r)["oidc_code"][0]
    r = client.post("/api/auth/oidc/exchange", json={"code": code})
    assert r.status_code == 200, r.data
    return r.get_json()


# ............ settings and methods ............ #


def test_defaults_keep_internal_login_only(monkeypatch):
    monkeypatch.delenv("OIDC_ISSUER", raising=False)
    monkeypatch.delenv("INTERNAL_LOGIN", raising=False)
    assert federated.login_methods() == {
        "internal": True,
        "oidc": False,
        "provider_name": None,
        "user_management": True,
    }


def test_methods_endpoint(provider, app):
    r = app.test_client().get("/api/auth/methods")
    assert r.get_json() == {
        "internal": False,
        "oidc": True,
        "provider_name": "Lab IdP",
        "user_management": False,
    }


def test_internal_login_only_blocks_when_disabled(monkeypatch):
    view = federated.internal_login_only(lambda: "ok")
    monkeypatch.setenv("INTERNAL_LOGIN", "false")
    body, status = view()
    assert status == 403
    monkeypatch.setenv("INTERNAL_LOGIN", "true")
    assert view() == "ok"


def test_role_map_parsing():
    assert federated.parse_role_map(
        "lab-admin:Admin, lab-admin:Organization_Admin,x:Bogus,junk"
    ) == {"lab-admin": {"Admin", "Organization_Admin"}}


# ............ browser login ............ #


@mongomock.patch(servers=MOCK_SERVERS)
def test_login_redirects_with_pkce_state_nonce(provider, app):
    mockdb()
    r = start_login(app.test_client())
    assert r.status_code == 302
    url = urlsplit(r.headers["Location"])
    assert url.netloc == "idp.example" and url.path == "/realms/lab/auth"
    q = parse_qs(url.query)
    assert q["client_id"] == [CLIENT_ID]
    assert q["redirect_uri"] == [CALLBACK]
    assert q["code_challenge_method"] == ["S256"]
    assert q["code_challenge"] and q["state"] and q["nonce"]
    assert q["scope"] == ["openid profile email offline_access"]
    stored = sessions.collection(sessions.STATES).find_one()
    assert stored["return_to"] == DASHBOARD and stored["verifier"]
    assert q["state"][0] not in json.dumps(stored, default=str)  # only its hash is stored


@mongomock.patch(servers=MOCK_SERVERS)
def test_redirect_uri_follows_request_host(provider, app):
    mockdb()
    r = start_login(app.test_client(), host="lab.tailnet.ts.net:10000")
    q = parse_qs(urlsplit(r.headers["Location"]).query)
    assert q["redirect_uri"] == ["http://lab.tailnet.ts.net:10000/cb"]


@pytest.mark.parametrize(
    "return_to",
    [
        None,
        "javascript:alert(1)",
        "https://evil.example/",
        "http://192.168.1.10@evil.example/",
        "ftp://192.168.1.10/",
    ],
)
@mongomock.patch(servers=MOCK_SERVERS)
def test_login_rejects_bad_return_to(provider, app, return_to):
    mockdb()
    r = app.test_client().get(
        "/api/auth/oidc/login", query_string={"return_to": return_to} if return_to else {}
    )
    assert r.status_code == 400


@mongomock.patch(servers=MOCK_SERVERS)
def test_login_allows_extra_hosts_any_port(provider, app, monkeypatch):
    mockdb()
    monkeypatch.setenv("OIDC_ALLOWED_RETURN_HOSTS", "dash.example.org")
    assert start_login(app.test_client(), "https://dash.example.org:8443/x").status_code == 302
    assert start_login(app.test_client(), "http://192.168.1.10:4200/").status_code == 302


@mongomock.patch(servers=MOCK_SERVERS)
def test_full_login_issues_oakestra_session(provider, app):
    mockdb()
    client = app.test_client()
    r, _ = browser_login(client, provider)
    assert r.status_code == 302
    assert r.headers["Location"].startswith(DASHBOARD + "#oidc_code=")

    # PKCE verifier and redirect_uri reached the token endpoint
    url, data, kwargs = provider.token_requests[-1]
    assert data["grant_type"] == "authorization_code" and data["code_verifier"]
    assert data["redirect_uri"] == CALLBACK

    code = fragment(r)["oidc_code"][0]
    r = client.post("/api/auth/oidc/exchange", json={"code": code})
    assert r.status_code == 200
    tokens = r.get_json()
    claims = decode(tokens["token"])
    assert claims["user"] == "ada@example.com"
    assert claims["roles"] == ["Admin"]
    assert claims["organization"] == root_id()
    assert claims["federated"] is True and "emergency" not in claims
    assert claims["exp"] - claims["iat"] == 15 * 60
    assert decode(tokens["refresh_token"])["type"] == "refresh"

    user = user_db.mongo_get_user_by_name("ada@example.com", root_id())
    assert user["oidc_iss"] == ISSUER and user["oidc_sub"] == "user-1"
    assert user["password"] is None
    assert user["roles"] == ["Admin"]
    session = sessions.collection(sessions.SESSIONS).find_one()
    assert session["refresh_token"] != "rt-1"  # encrypted at rest
    assert sessions.decrypt_token(session["refresh_token"], session["_id"]) == "rt-1"


@mongomock.patch(servers=MOCK_SERVERS)
def test_one_time_code_cannot_be_reused(provider, app):
    mockdb()
    client = app.test_client()
    r, _ = browser_login(client, provider)
    code = fragment(r)["oidc_code"][0]
    assert client.post("/api/auth/oidc/exchange", json={"code": code}).status_code == 200
    r = client.post("/api/auth/oidc/exchange", json={"code": code})
    assert r.status_code == 400 and r.get_json()["message"]
    assert client.post("/api/auth/oidc/exchange", json={}).status_code == 400


@mongomock.patch(servers=MOCK_SERVERS)
def test_expired_code_is_rejected(provider, app):
    mockdb()
    code = sessions.create_code(
        {"username": "x", "roles": ["Admin"], "sid": "s"}, ttl=timedelta(seconds=-1)
    )
    assert app.test_client().post("/api/auth/oidc/exchange", json={"code": code}).status_code == 400


@mongomock.patch(servers=MOCK_SERVERS)
def test_state_is_single_use(provider, app):
    mockdb()
    client = app.test_client()
    r, query = browser_login(client, provider)
    assert "oidc_code" in fragment(r)
    r = client.get(
        "/api/auth/oidc/callback", query_string={"code": "c1", "state": query["state"][0]}
    )
    assert r.status_code == 400
    assert b"invalid_request" in r.data


@mongomock.patch(servers=MOCK_SERVERS)
def test_unknown_state_renders_plain_error(provider, app):
    mockdb()
    r = app.test_client().get(
        "/api/auth/oidc/callback", query_string={"code": "c", "state": "nope"}
    )
    assert r.status_code == 400 and r.mimetype == "text/plain"
    assert b"invalid_request" in r.data


@pytest.mark.parametrize(
    "tamper,expected",
    [
        ({"nonce": "other"}, "provider_error"),
        ({"iss": "https://evil.example"}, "provider_error"),
        ({"aud": "other-client"}, "provider_error"),
        ({"exp": int(time.time()) - 3600, "iat": int(time.time()) - 4000}, "provider_error"),
    ],
)
@mongomock.patch(servers=MOCK_SERVERS)
def test_callback_rejects_invalid_id_tokens(provider, app, tamper, expected):
    mockdb()
    client = app.test_client()
    r = start_login(client)
    q = parse_qs(urlsplit(r.headers["Location"]).query)
    provider.nonce = q["nonce"][0]
    original = provider.id_token
    provider.id_token = lambda **kw: original(**{**tamper, **kw})
    r = client.get("/api/auth/oidc/callback", query_string={"code": "c", "state": q["state"][0]})
    assert fragment(r) == {"oidc_error": [expected]}
    assert user_db.mongo_get_user_by_name("ada@example.com") is None


@mongomock.patch(servers=MOCK_SERVERS)
def test_callback_rejects_token_signed_by_another_key(provider, app):
    mockdb()
    client = app.test_client()
    r = start_login(client)
    q = parse_qs(urlsplit(r.headers["Location"]).query)
    provider.nonce = q["nonce"][0]
    good_key = provider.key
    provider.key = JsonWebKey.generate_key("RSA", 2048, is_private=True, options={"kid": "k1"})
    token = provider.id_token()
    provider.key = good_key
    original_post = provider.post

    def post(session, url, data=None, **kwargs):
        resp = original_post(session, url, data, **kwargs)
        resp._body["id_token"] = token
        return resp

    provider.post = post
    r = client.get("/api/auth/oidc/callback", query_string={"code": "c", "state": q["state"][0]})
    assert fragment(r) == {"oidc_error": ["provider_error"]}


@mongomock.patch(servers=MOCK_SERVERS)
def test_callback_provider_errors(provider, app):
    mockdb()
    client = app.test_client()
    for args, expected in (
        ({"error": "access_denied"}, "access_denied"),
        ({"error": "server_error"}, "provider_error"),
        ({}, "invalid_request"),
    ):
        state = parse_qs(urlsplit(start_login(client).headers["Location"]).query)["state"][0]
        r = client.get("/api/auth/oidc/callback", query_string={"state": state, **args})
        assert fragment(r) == {"oidc_error": [expected]}
    provider.token_error = (400, {"error": "invalid_grant"})
    r, _ = browser_login(client, provider)
    assert fragment(r) == {"oidc_error": ["provider_error"]}
    provider.token_error = requests.ConnectionError("down")
    r, _ = browser_login(client, provider)
    assert fragment(r) == {"oidc_error": ["provider_error"]}


# ............ roles ............ #


@mongomock.patch(servers=MOCK_SERVERS)
def test_no_oakestra_role_is_access_denied(provider, app):
    mockdb()
    provider.roles = ["viewer", "Bogus"]
    r, _ = browser_login(app.test_client(), provider)
    assert fragment(r) == {"oidc_error": ["access_denied"]}
    assert user_db.mongo_get_user_by_name("ada@example.com") is None


@mongomock.patch(servers=MOCK_SERVERS)
def test_role_claim_path_and_map(provider, app, monkeypatch):
    mockdb()
    monkeypatch.setenv("OIDC_ROLES_CLAIM", "resource_access.oakestra-lab.roles")
    monkeypatch.setenv(
        "OIDC_ROLE_MAP", "lab-admin:Admin,lab-admin:Organization_Admin,dev:Application_Provider"
    )
    provider.claims["resource_access"] = {
        "oakestra-lab": {"roles": ["lab-admin", "unknown", "Infrastructure_Provider"]}
    }
    tokens = login_tokens(app.test_client(), provider)
    assert decode(tokens["token"])["roles"] == [
        "Admin",
        "Organization_Admin",
        "Infrastructure_Provider",
    ]


@mongomock.patch(servers=MOCK_SERVERS)
def test_roles_from_userinfo_when_missing_in_id_token(provider, app, monkeypatch):
    mockdb()
    monkeypatch.setenv("OIDC_ROLES_CLAIM", "groups")
    provider.userinfo = {"sub": "user-1", "groups": ["Application_Provider"]}
    tokens = login_tokens(app.test_client(), provider)
    assert decode(tokens["token"])["roles"] == ["Application_Provider"]


@mongomock.patch(servers=MOCK_SERVERS)
def test_userinfo_for_another_subject_is_rejected(provider, app, monkeypatch):
    mockdb()
    monkeypatch.setenv("OIDC_ROLES_CLAIM", "groups")
    provider.userinfo = {"sub": "someone-else", "groups": ["Admin"]}
    r, _ = browser_login(app.test_client(), provider)
    assert fragment(r) == {"oidc_error": ["provider_error"]}


def test_map_roles_accepts_strings(monkeypatch):
    monkeypatch.delenv("OIDC_ROLE_MAP", raising=False)
    assert oidc.map_roles("Admin Application_Provider") == ["Admin", "Application_Provider"]
    assert oidc.map_roles(None) == []
    assert oidc.map_roles({"Admin": True}) == []


def test_mongo_add_user_role_replaces_roles():
    """The login role sync relies on this helper REPLACING (not appending) the member's roles."""
    with mongomock.patch(servers=MOCK_SERVERS):
        mockdb()
        user_id = str(user_db.mongo_get_user_by_name("Admin")["_id"])
        organization_db.mongo_add_user_role_to_organization(
            user_id, root_id(), ["Admin", "Application_Provider"]
        )
        organization_db.mongo_add_user_role_to_organization(
            user_id, root_id(), ["Application_Provider"]
        )
        members = organization_db.mongo_get_organization_by_name("root")["member"]
        assert [m["roles"] for m in members if m["user_id"] == user_id] == [
            ["Application_Provider"]
        ]


@mongomock.patch(servers=MOCK_SERVERS)
def test_roles_are_replaced_on_every_login(provider, app):
    mockdb()
    client = app.test_client()
    login_tokens(client, provider)
    provider.roles = ["Application_Provider"]
    login_tokens(client, provider)
    assert len(user_db.mongo_get_user()) == 2  # Admin + ada, no duplicate
    assert roles_of("ada@example.com") == ["Application_Provider"]
    provider.roles = []
    r, _ = browser_login(client, provider)
    assert fragment(r) == {"oidc_error": ["access_denied"]}
    assert roles_of("ada@example.com") is None  # membership removed


# ............ user keying ............ #


@mongomock.patch(servers=MOCK_SERVERS)
def test_username_is_fixed_at_first_login(provider, app):
    mockdb()
    client = app.test_client()
    login_tokens(client, provider)
    provider.claims.update(
        preferred_username="ada.lovelace@new.example", email="ada.lovelace@new.example"
    )
    tokens = login_tokens(client, provider)
    assert decode(tokens["token"])["user"] == "ada@example.com"
    user = user_db.mongo_get_user_by_name("ada@example.com")
    assert user["email"] == "ada.lovelace@new.example"


@mongomock.patch(servers=MOCK_SERVERS)
def test_same_name_other_subject_gets_suffix(provider, app):
    mockdb()
    client = app.test_client()
    login_tokens(client, provider)
    provider.sub = "user-2"
    tokens = login_tokens(client, provider)
    name = decode(tokens["token"])["user"]
    assert name.startswith("ada@example.com-") and len(name) == len("ada@example.com-") + 4
    assert len(user_db.mongo_get_user()) == 3


@mongomock.patch(servers=MOCK_SERVERS)
def test_cannot_take_over_local_user(provider, app):
    mockdb()
    provider.claims["preferred_username"] = "Admin"
    r, _ = browser_login(app.test_client(), provider)
    assert fragment(r) == {"oidc_error": ["user_conflict"]}
    admin = user_db.mongo_get_user_by_name("Admin")
    assert "oidc_sub" not in admin


@mongomock.patch(servers=MOCK_SERVERS)
def test_username_fallbacks(provider, app):
    mockdb()
    del provider.claims["preferred_username"]
    assert decode(login_tokens(app.test_client(), provider)["token"])["user"] == "ada@example.com"
    provider.sub = "user-9"
    provider.claims = {}
    assert decode(login_tokens(app.test_client(), provider)["token"])["user"] == "user-9"


@mongomock.patch(servers=MOCK_SERVERS)
def test_password_login_rejects_federated_user(provider, app):
    from users.auth import user_login

    mockdb()
    login_tokens(app.test_client(), provider)
    with patch("users.auth.securityUtils") as su:
        assert user_login({"username": "ada@example.com", "password": "anything"}) == {}
        su.create_jwt_auth_access_token.assert_not_called()


# ............ renewal ............ #


def refresh(client, tokens):
    return client.post("/api/auth/refresh", headers=bearer(tokens["refresh_token"]))


def age_session(seconds):
    sessions.collection(sessions.SESSIONS).update_many(
        {}, {"$set": {"confirmed_at": sessions.now() - timedelta(seconds=seconds)}}
    )


@mongomock.patch(servers=MOCK_SERVERS)
def test_refresh_ok_rereads_roles_and_rotates(provider, app):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    age_session(120)
    provider.roles = ["Admin", "Infrastructure_Provider"]
    r = refresh(client, tokens)
    assert r.status_code == 200
    assert set(r.get_json()) == {"token"}
    assert decode(r.get_json()["token"])["roles"] == ["Admin", "Infrastructure_Provider"]
    assert roles_of("ada@example.com") == ["Admin", "Infrastructure_Provider"]
    url, data, _ = provider.token_requests[-1]
    assert data["grant_type"] == "refresh_token" and data["refresh_token"] == "rt-1"
    session = sessions.collection(sessions.SESSIONS).find_one()
    assert sessions.decrypt_token(session["refresh_token"], session["_id"]) == "rt-2"


@mongomock.patch(servers=MOCK_SERVERS)
def test_refresh_recently_confirmed_does_not_call_provider(provider, app):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    calls = len(provider.token_requests)
    assert refresh(client, tokens).status_code == 200
    assert len(provider.token_requests) == calls


@mongomock.patch(servers=MOCK_SERVERS)
def test_refresh_roles_from_userinfo_without_id_token(provider, app):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    age_session(120)
    provider.include_id_token = False
    provider.userinfo = {"sub": "user-1", "oakestra_roles": ["Application_Provider"]}
    r = refresh(client, tokens)
    assert decode(r.get_json()["token"])["roles"] == ["Application_Provider"]


@pytest.mark.parametrize(
    "error",
    [
        (400, {"error": "invalid_grant"}),
        (401, {"error": "invalid_client"}),
        (400, ValueError("html")),
    ],
)
@mongomock.patch(servers=MOCK_SERVERS)
def test_refresh_refused_is_401(provider, app, error):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    age_session(120)
    provider.token_error = error
    assert refresh(client, tokens).status_code == 401
    assert sessions.collection(sessions.SESSIONS).count_documents({}) == 0
    provider.token_error = None
    assert refresh(client, tokens).status_code == 401  # session is gone


@mongomock.patch(servers=MOCK_SERVERS)
def test_refresh_without_roles_is_401_and_removes_roles(provider, app):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    age_session(120)
    provider.roles = []
    assert refresh(client, tokens).status_code == 401
    assert roles_of("ada@example.com") is None


@pytest.mark.parametrize(
    "outage", [requests.ConnectionError("down"), requests.Timeout("slow"), (503, {"error": "x"})]
)
@mongomock.patch(servers=MOCK_SERVERS)
def test_refresh_provider_unreachable_within_grace(provider, app, outage):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    age_session(3600)
    provider.token_error = outage
    r = refresh(client, tokens)
    assert r.status_code == 200
    assert decode(r.get_json()["token"])["roles"] == ["Admin"]


@mongomock.patch(servers=MOCK_SERVERS)
def test_refresh_provider_unreachable_after_grace(provider, app, monkeypatch):
    mockdb()
    monkeypatch.setenv("FEDERATED_OUTAGE_GRACE_HOURS", "1")
    client = app.test_client()
    tokens = login_tokens(client, provider)
    age_session(2 * 3600)
    provider.token_error = requests.ConnectionError("down")
    assert refresh(client, tokens).status_code == 401


@mongomock.patch(servers=MOCK_SERVERS)
def test_refresh_concurrent_renewal_reuses_roles(provider, app):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    age_session(120)
    sid = decode(tokens["token"])["sid"]
    assert sessions.lock_session(sid)
    calls = len(provider.token_requests)
    assert refresh(client, tokens).status_code == 200
    assert len(provider.token_requests) == calls


@mongomock.patch(servers=MOCK_SERVERS)
def test_session_minutes_setting(provider, app, monkeypatch):
    mockdb()
    monkeypatch.setenv("FEDERATED_SESSION_MINUTES", "5")
    claims = decode(login_tokens(app.test_client(), provider)["token"])
    assert claims["exp"] - claims["iat"] == 300


# ............ sign-out ............ #


def admin_token():
    return local_access_token(
        "Admin", {"user": "Admin", "roles": ["Admin"], "organization": root_id()}
    )


@mongomock.patch(servers=MOCK_SERVERS)
def test_signout_user_rejects_old_tokens(provider, app):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    time.sleep(1.05)  # iat has 1 s resolution
    admin = admin_token()
    r = client.post("/api/users/ada@example.com/signout", headers=bearer(admin))
    assert r.status_code == 200
    assert sessions.collection(sessions.SESSIONS).count_documents({}) == 0
    assert (
        client.get("/api/user/ada@example.com", headers=bearer(tokens["token"])).status_code == 401
    )
    assert refresh(client, tokens).status_code == 401
    # the Admin is not affected, and a new sign-in works
    assert client.get("/api/users/", headers=bearer(admin)).status_code == 200
    time.sleep(1.05)
    new = login_tokens(client, provider)
    assert client.get("/api/user/ada@example.com", headers=bearer(new["token"])).status_code == 200


@mongomock.patch(servers=MOCK_SERVERS)
def test_signout_requires_admin_and_known_user(provider, app):
    mockdb()
    client = app.test_client()
    provider.roles = ["Application_Provider"]
    tokens = login_tokens(client, provider)
    assert client.post("/api/auth/signout-all", headers=bearer(tokens["token"])).status_code == 403
    r = client.post("/api/users/nobody/signout", headers=bearer(admin_token()))
    assert r.status_code == 404


@mongomock.patch(servers=MOCK_SERVERS)
def test_signout_all(provider, app):
    mockdb()
    client = app.test_client()
    tokens = login_tokens(client, provider)
    admin = admin_token()
    time.sleep(1.05)
    assert client.post("/api/auth/signout-all", headers=bearer(admin)).status_code == 200
    assert (
        client.get("/api/user/ada@example.com", headers=bearer(tokens["token"])).status_code == 401
    )
    assert client.get("/api/users/", headers=bearer(admin)).status_code == 401
    assert sessions.collection(sessions.SESSIONS).count_documents({}) == 0


@mongomock.patch(servers=MOCK_SERVERS)
def test_revocation_check_is_cached(provider, app):
    mockdb()
    payload = {"iat": int(time.time()), "sub": "x"}
    assert sessions.token_revoked({}, payload) is False
    with patch.object(sessions, "collection", side_effect=AssertionError("no db hit")):
        for _ in range(100):
            assert sessions.token_revoked({}, payload) is False


@mongomock.patch(servers=MOCK_SERVERS)
def test_signout_feed(provider, app, monkeypatch):
    mockdb()
    monkeypatch.setenv("OIDC_SIGNOUT_FEED_URL", ISSUER + "/clusters/signouts")
    client = app.test_client()
    login_tokens(client, provider)
    calls = []

    def feed(url, params=None, auth=None, timeout=None):
        calls.append((url, params, auth))
        return FakeResponse(
            body={
                "signouts": [{"sub": "user-1", "at": "2099-01-01T00:00:00Z"}, {"sub": "unknown"}],
                "now": "2026-10-30T12:00:00Z",
            }
        )

    signout_feed._state["since"] = None
    with patch("users.signout_feed.requests.get", side_effect=feed):
        assert signout_feed.poll_once() == 1
        assert signout_feed.poll_once() == 1
    assert calls[0][1] == {} and calls[1][1] == {"since": "2026-10-30T12:00:00Z"}
    assert calls[0][2] == (CLIENT_ID, "s3cret")
    assert sessions.collection(sessions.SESSIONS).count_documents({}) == 0
    assert sessions.token_revoked({}, {"iat": int(time.time()), "sub": "ada@example.com"})


def test_signout_feed_off_without_url(monkeypatch):
    monkeypatch.delenv("OIDC_SIGNOUT_FEED_URL", raising=False)
    assert signout_feed.start() is None


# ............ management lock ............ #


LOCKED = [
    ("put", "/api/user/someone", {"email": "x"}),
    ("delete", "/api/user/someone", None),
    ("post", "/api/auth/register", {"name": "x", "password": "y", "roles": ["Admin"]}),
    ("post", "/api/organization/", {"name": "org"}),
    ("put", "/api/organization/0123456789ab0123456789ab", {"name": "org"}),
    ("delete", "/api/organization/0123456789ab0123456789ab", None),
]


@pytest.mark.parametrize("method,url,body", LOCKED)
@mongomock.patch(servers=MOCK_SERVERS)
def test_management_writes_locked(provider, app, method, url, body):
    mockdb()
    r = getattr(app.test_client(), method)(url, json=body, headers=bearer(admin_token()))
    assert r.status_code == 403
    assert r.get_json()["message"] == "Users and roles are managed by your identity provider"


@mongomock.patch(servers=MOCK_SERVERS)
def test_management_reads_stay_and_unlock_with_internal_login(provider, app, monkeypatch):
    mockdb()
    client = app.test_client()
    admin = admin_token()
    assert client.get("/api/users/", headers=bearer(admin)).status_code == 200
    assert client.get("/api/organization/", headers=bearer(admin)).status_code == 200
    monkeypatch.setenv("INTERNAL_LOGIN", "true")
    r = client.post("/api/organization/", json={"name": "org", "member": []}, headers=bearer(admin))
    assert r.status_code == 200


# ............ emergency ............ #


@mongomock.patch(servers=MOCK_SERVERS)
def test_emergency_login(provider, app, capsys):
    mockdb()
    with patch.object(mongodb_client, "mongo_init_standalone"):
        assert emergency.main(["--minutes", "30"]) == 0
    out = json.loads(capsys.readouterr().out.strip())
    assert out["expires_in"] == 600 and out["session_minutes"] == 30
    client = app.test_client()
    r = client.post("/api/auth/oidc/exchange", json={"code": out["code"]})
    assert r.status_code == 200
    tokens = r.get_json()
    claims = decode(tokens["token"])
    assert claims["user"] == "emergency-admin" and claims["emergency"] is True
    assert "Admin" in claims["roles"]
    assert claims["exp"] - claims["iat"] == 30 * 60
    assert decode(tokens["refresh_token"])["exp"] - claims["iat"] == 30 * 60
    user = user_db.mongo_get_user_by_name("emergency-admin", root_id())
    assert user["password"] is None and "oidc_sub" not in user and "Admin" in user["roles"]
    # renewal never goes past the end of the session
    r = refresh(client, tokens)
    renewed = decode(r.get_json()["token"])
    assert renewed["emergency"] is True and renewed["exp"] <= claims["exp"] + 1
    sessions.collection(sessions.SESSIONS).update_many(
        {}, {"$set": {"expires_at": sessions.now() - timedelta(seconds=1)}}
    )
    assert refresh(client, tokens).status_code == 401
    # the code is single use
    assert client.post("/api/auth/oidc/exchange", json={"code": out["code"]}).status_code == 400


@mongomock.patch(servers=MOCK_SERVERS)
def test_emergency_rejects_bad_minutes_and_federated_name(provider, capsys):
    mockdb()
    with patch.object(mongodb_client, "mongo_init_standalone"):
        assert emergency.main(["--minutes", "0"]) == 1
        user_db.mongo_save_user_without_roles(
            {"name": "emergency-admin", "oidc_iss": ISSUER, "oidc_sub": "x"}
        )
        assert emergency.main(["--minutes", "5"]) == 1


# ............ device login ............ #


@mongomock.patch(servers=MOCK_SERVERS)
def test_device_login(provider, app):
    mockdb()
    client = app.test_client()
    r = client.post("/api/auth/oidc/device")
    assert r.status_code == 200
    assert r.get_json() == {
        "device_code": "dev-1",
        "user_code": "ABCD-EFGH",
        "verification_uri": ISSUER + "/device-ui",
        "verification_uri_complete": ISSUER + "/device-ui?code=ABCD-EFGH",
        "expires_in": 600,
        "interval": 5,
    }
    url, data, kwargs = provider.token_requests[-1]
    assert url == ISSUER + "/device" and data["client_id"] == CLIENT_ID
    assert kwargs["auth"] is not None  # client_secret_basic

    for error in ("authorization_pending", "slow_down", "access_denied", "expired_token"):
        provider.token_error = (400, {"error": error})
        r = client.post("/api/auth/oidc/device/token", json={"device_code": "dev-1"})
        assert r.status_code == 400 and r.get_json()["error"] == error

    provider.token_error = None
    r = client.post("/api/auth/oidc/device/token", json={"device_code": "dev-1"})
    assert r.status_code == 200
    assert decode(r.get_json()["token"])["user"] == "ada@example.com"
    url, data, _ = provider.token_requests[-1]
    assert data["grant_type"] == "urn:ietf:params:oauth:grant-type:device_code"
    assert data["device_code"] == "dev-1"


@mongomock.patch(servers=MOCK_SERVERS)
def test_device_login_without_roles_or_endpoint(provider, app):
    mockdb()
    client = app.test_client()
    provider.roles = []
    r = client.post("/api/auth/oidc/device/token", json={"device_code": "dev-1"})
    assert r.status_code == 400 and r.get_json()["error"] == "access_denied"
    provider.token_error = requests.ConnectionError("down")
    assert client.post("/api/auth/oidc/device/token", json={"device_code": "d"}).status_code == 502
    del provider.discovery["device_authorization_endpoint"]
    oidc.reset_cache()
    assert client.post("/api/auth/oidc/device").status_code == 404


def test_oidc_disabled(monkeypatch, app):
    monkeypatch.delenv("OIDC_ISSUER", raising=False)
    client = app.test_client()
    assert (
        client.get("/api/auth/oidc/login", query_string={"return_to": DASHBOARD}).status_code == 404
    )
    assert client.post("/api/auth/oidc/device").status_code == 404
