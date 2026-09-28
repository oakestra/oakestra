"""Tests for the jwt-generator Flask service: token issuance, key exposure, and health."""

import jwt
import pytest


def decode(token, public_key):
    return jwt.decode(token, public_key, algorithms=["RS256"])


# --- GET /key -----------------------------------------------------------


def test_get_key_with_generated_keys_returns_pem_public_key(fresh_module):
    client = fresh_module().app.test_client()

    response = client.get("/key")

    assert response.status_code == 200
    key = response.get_json()["public_key"]
    assert key.startswith("-----BEGIN PUBLIC KEY-----")
    assert key.strip().endswith("-----END PUBLIC KEY-----")


def test_get_key_with_env_keys_returns_exact_env_key(app_env_keys, rsa_keypair):
    _, expected_public_pem = rsa_keypair
    client = app_env_keys.app.test_client()

    response = client.get("/key")

    assert response.status_code == 200
    assert response.get_json()["public_key"] == expected_public_pem


# --- POST /create ---------------------------------------------------------


def test_create_access_token_verifies_against_published_key(client):
    response = client.post("/create", json={"identity": "user-42"})

    assert response.status_code == 200
    token = response.get_json()["access_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["sub"] == "user-42"
    assert claims["type"] == "access"


def test_create_access_token_additional_claims_present(client):
    response = client.post(
        "/create",
        json={"identity": "user-42", "additional_claims": {"role": "admin", "org": "root"}},
    )

    token = response.get_json()["access_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["role"] == "admin"
    assert claims["org"] == "root"


def test_create_access_token_default_expiry_is_about_ten_minutes(client):
    response = client.post("/create", json={"identity": "user-42"})

    token = response.get_json()["access_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["exp"] - claims["iat"] == 600


def test_create_access_token_expires_delta_honoured(client):
    response = client.post(
        "/create",
        json={
            "identity": "user-42",
            "expires_delta": {"days": 0, "seconds": 3600, "microseconds": 0},
        },
    )

    token = response.get_json()["access_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["exp"] - claims["iat"] == 3600


def test_create_access_token_fresh_flag_honoured(client):
    # parse_timedelta only accepts dict-shaped values (see the bug test below for
    # the plain-boolean case), so freshness has to be requested as a duration.
    response = client.post(
        "/create",
        json={"identity": "user-42", "fresh": {"days": 0, "seconds": 120, "microseconds": 0}},
    )

    token = response.get_json()["access_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    # a timedelta "fresh" becomes a fresh-until unix timestamp: iat + the requested
    # duration (flask_jwt_extended's _encode_jwt does datetime.timestamp(now + fresh)).
    assert claims["fresh"] == pytest.approx(claims["iat"] + 120, abs=2)


def test_create_access_token_without_fresh_is_not_fresh(client):
    response = client.post("/create", json={"identity": "user-42"})

    token = response.get_json()["access_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["fresh"] is False


def test_create_access_token_additional_headers_land_in_jwt_header(client):
    response = client.post(
        "/create",
        json={"identity": "user-42", "additional_headers": {"kid": "my-key-id"}},
    )

    token = response.get_json()["access_token"]
    header = jwt.get_unverified_header(token)
    assert header["kid"] == "my-key-id"


def test_create_non_json_body_returns_400(client):
    response = client.post("/create", data="not json", content_type="text/plain")

    assert response.status_code == 400
    assert response.get_json() == {"msg": "Missing JSON in request"}


@pytest.mark.xfail(
    strict=True,
    reason=(
        "jwt_generator.py parse_timedelta ignores a non-dict 'fresh', so fresh=True "
        "yields a non-fresh token. Not reachable today: the system manager only ever "
        "sends a timedelta dict or leaves the field out."
    ),
)
def test_create_access_token_boolean_fresh_true_is_honoured(client):
    response = client.post("/create", json={"identity": "user-42", "fresh": True})

    token = response.get_json()["access_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["fresh"] is True


# --- POST /refresh ----------------------------------------------------------


def test_refresh_token_returns_type_refresh_with_claims(client):
    response = client.post(
        "/refresh", json={"identity": "user-42", "additional_claims": {"role": "viewer"}}
    )

    assert response.status_code == 200
    token = response.get_json()["refresh_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["sub"] == "user-42"
    assert claims["type"] == "refresh"
    assert claims["role"] == "viewer"


def test_refresh_token_default_expiry_is_thirty_days(client):
    response = client.post("/refresh", json={"identity": "user-42"})

    token = response.get_json()["refresh_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["exp"] - claims["iat"] == 30 * 24 * 3600


def test_refresh_non_json_body_returns_400(client):
    response = client.post("/refresh", data="not json", content_type="text/plain")

    assert response.status_code == 400
    assert response.get_json() == {"msg": "Missing JSON in request"}


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: jwt_generator.py create_refresh_token_route re-wraps an already-parsed "
        "timedelta in timedelta(minutes=expires_delta), which raises TypeError and "
        "surfaces as a 500 instead of honouring the requested expiry (see "
        "system-manager-python/ext_requests/jwt_generator_requests.py "
        "create_refresh_token, which sends expires_delta as {days,seconds,microseconds})"
    ),
)
def test_refresh_token_expires_delta_honoured(client):
    response = client.post(
        "/refresh",
        json={"identity": "user-42", "expires_delta": {"days": 1, "seconds": 0, "microseconds": 0}},
    )

    assert response.status_code == 200
    token = response.get_json()["refresh_token"]
    public_key = client.get("/key").get_json()["public_key"]
    claims = decode(token, public_key)
    assert claims["exp"] - claims["iat"] == 24 * 3600


# --- GET /health --------------------------------------------------------


def test_get_health_returns_working(client):
    response = client.get("/health")

    assert response.status_code == 200
    assert response.get_data(as_text=True) == "Working!"


# --- Key isolation between instances --------------------------------------


def test_two_generated_instances_produce_different_keys(fresh_module):
    first = fresh_module()
    second = fresh_module()

    assert first.public_key != second.public_key

    first_client = first.app.test_client()
    token = first_client.post("/create", json={"identity": "user-42"}).get_json()["access_token"]

    # Without keys from the env, every instance generates its own pair, so tokens from
    # one must not verify against another.
    with pytest.raises(jwt.exceptions.InvalidSignatureError):
        decode(token, second.public_key)
