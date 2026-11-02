from unittest import mock

import db.mongodb_client as mongodb_client
import docker
import pytest
from api.v1.schema import MarketplaceAddonSchema
from db import marketplace_db
from marshmallow import ValidationError
from services import marketplace_service


def addon(**overrides):
    base = {
        "name": "my-addon",
        "services": [
            {"service_name": "backend", "image": "img/backend:1"},
            {"service_name": "web", "image": "img/web:1"},
        ],
    }
    return {**base, **overrides}


UI = {"service": "web", "sdk": "^0.1.0"}


def test_schema_accepts_and_dumps_ui():
    schema = MarketplaceAddonSchema()
    loaded = schema.load(addon(ui=UI))
    assert schema.dump(loaded)["ui"] == UI


def test_schema_without_ui_dumps_no_ui_key():
    schema = MarketplaceAddonSchema()
    assert "ui" not in schema.dump(schema.load(addon()))


def test_schema_rejects_ui_service_not_in_services():
    with pytest.raises(ValidationError) as exc:
        MarketplaceAddonSchema().load(addon(ui={"service": "nope", "sdk": "^0.1.0"}))
    assert "ui" in exc.value.messages


def test_schema_rejects_bad_name_when_ui_set():
    with pytest.raises(ValidationError) as exc:
        MarketplaceAddonSchema().load(addon(name="My Addon", ui=UI))
    assert "name" in exc.value.messages


def test_schema_accepts_any_name_without_ui():
    assert MarketplaceAddonSchema().load(addon(name="My Addon"))["name"] == "My Addon"


def test_schema_rejects_empty_sdk():
    with pytest.raises(ValidationError):
        MarketplaceAddonSchema().load(addon(ui={"service": "web", "sdk": ""}))


def test_schema_still_rejects_unknown_fields():
    with pytest.raises(ValidationError) as exc:
        MarketplaceAddonSchema().load(addon(bogus=1))
    assert "bogus" in exc.value.messages


@pytest.fixture
def docker_client(monkeypatch):
    fake = mock.MagicMock()
    monkeypatch.setattr(marketplace_service.docker, "from_env", lambda: fake)
    return fake


@pytest.fixture
def update_addon(monkeypatch):
    fake = mock.MagicMock()
    monkeypatch.setattr(marketplace_db, "update_addon", fake)
    return fake


def test_verify_addon_fails_when_second_pull_fails(docker_client, update_addon):
    docker_client.images.pull.side_effect = [mock.MagicMock(id="a"), docker.errors.APIError("x")]
    marketplace_service.verify_addon("id1", addon())

    statuses = [c.args[1]["status"] for c in update_addon.call_args_list]
    assert statuses == ["verification_failed"]


def test_verify_addon_approves_once_after_all_pulls(docker_client, update_addon):
    docker_client.images.pull.return_value = mock.MagicMock(id="a")
    marketplace_service.verify_addon("id1", addon())

    assert docker_client.images.pull.call_count == 2
    update_addon.assert_called_once_with("id1", {"status": "approved"})


@pytest.mark.parametrize("method", ["get", "delete"])
def test_invalid_id_returns_404(client, monkeypatch, method):
    # Any Mongo access raises here, so a 404 means the id check ran first.
    monkeypatch.setattr(mongodb_client, "mongo_marketplace", mock.MagicMock(side_effect=Exception))
    resp = getattr(client, method)("/api/v1/marketplace/addons/not-an-id")
    assert resp.status_code == 404


def test_db_guards_invalid_id():
    assert marketplace_db.find_addon_by_id("not-an-id") is None
    assert marketplace_db.update_addon("not-an-id", {}) is None
    assert marketplace_db.delete_addon_by_id("not-an-id") is False


def test_post_with_ui_returns_201_with_ui(client, monkeypatch):
    registered = {**addon(ui=UI), "_id": "abc", "status": "under_review"}
    register = mock.MagicMock(return_value=registered)
    monkeypatch.setattr(marketplace_service, "register_addon", register)

    resp = client.post("/api/v1/marketplace/addons/", json=addon(ui=UI))
    assert resp.status_code == 201
    assert resp.get_json()["ui"] == UI
    register.assert_called_once()


def test_post_rejects_bad_ui_with_422(client, monkeypatch):
    monkeypatch.setattr(marketplace_service, "register_addon", mock.MagicMock())
    resp = client.post(
        "/api/v1/marketplace/addons/", json=addon(ui={"service": "nope", "sdk": "^0.1.0"})
    )
    assert resp.status_code == 422
