from datetime import datetime
from unittest.mock import MagicMock

import pytest
import requests
from bson.objectid import ObjectId
from services import addons_service

ADDON_ID = ObjectId()
# pymongo gives us naive datetimes, so the fixtures use them too.
CREATED_AT = datetime(2026, 11, 2, 9, 30, 0)


def _stored_addon(status="active"):
    return {
        "_id": ADDON_ID,
        "marketplace_id": "mp-1",
        "name": "metrics",
        "status": status,
        "services": [],
        "created_at": CREATED_AT,
        "updated_at": CREATED_AT,
    }


def test_list_returns_json_with_string_ids_and_iso_timestamps(client, addons_collection):
    addons_collection.docs.append(_stored_addon())

    response = client.get("/api/v1/addons/")

    assert response.status_code == 200
    assert response.mimetype == "application/json"
    [addon] = response.get_json()
    assert addon["_id"] == str(ADDON_ID)
    assert addon["created_at"] == "2026-11-02T09:30:00+00:00"
    assert addon["updated_at"] == "2026-11-02T09:30:00+00:00"


def test_list_filters_by_status(client, addons_collection):
    addons_collection.docs.append(_stored_addon(status="installing"))

    assert client.get("/api/v1/addons/?status=installing").get_json()[0]["name"] == "metrics"
    assert client.get("/api/v1/addons/?status=active").get_json() == []


def test_get_returns_json(client, addons_collection):
    addons_collection.docs.append(_stored_addon())

    response = client.get(f"/api/v1/addons/{ADDON_ID}")

    assert response.status_code == 200
    assert response.mimetype == "application/json"
    assert response.get_json()["_id"] == str(ADDON_ID)


def test_patch_updates_status_and_extra_fields(client, addons_collection):
    addons_collection.docs.append(_stored_addon(status="installing"))

    response = client.patch(
        f"/api/v1/addons/{ADDON_ID}", json={"status": "active", "status_details": "ok"}
    )

    assert response.status_code == 200
    body = response.get_json()
    assert body["status"] == "active"
    assert body["status_details"] == "ok"
    assert body["updated_at"] != body["created_at"]


@pytest.mark.parametrize("addon_id", ["not-an-object-id", str(ObjectId())])
def test_unknown_or_invalid_id_is_404(client, addons_collection, addon_id):
    assert client.get(f"/api/v1/addons/{addon_id}").status_code == 404
    assert client.patch(f"/api/v1/addons/{addon_id}", json={"status": "active"}).status_code == 404
    assert client.delete(f"/api/v1/addons/{addon_id}").status_code == 404


def test_delete_moves_enabled_addon_to_disabling(client, addons_collection):
    addons_collection.docs.append(_stored_addon())

    response = client.delete(f"/api/v1/addons/{ADDON_ID}")

    assert response.status_code == 200
    assert response.mimetype == "application/json"
    assert response.get_json()["status"] == "disabling"
    assert len(addons_collection.docs) == 1


def test_delete_removes_disabled_addon(client, addons_collection):
    addons_collection.docs.append(_stored_addon(status="disabled"))

    response = client.delete(f"/api/v1/addons/{ADDON_ID}")

    assert response.status_code == 204
    assert addons_collection.docs == []


def _marketplace_response(status_code, body=None):
    response = MagicMock(status_code=status_code)
    response.json.return_value = body
    if status_code >= 400:
        response.raise_for_status.side_effect = requests.HTTPError(f"{status_code}")
    return response


def test_post_installs_addon(client, addons_collection, monkeypatch):
    marketplace_addon = {
        "name": "metrics",
        "services": [{"service_name": "ui", "image": "ui:1.0"}],
        "ui": {"service": "ui", "sdk": "^1.0.0"},
    }
    monkeypatch.setattr(
        addons_service.requests,
        "get",
        lambda *a, **kw: _marketplace_response(200, marketplace_addon),
    )

    response = client.post("/api/v1/addons/", json={"marketplace_id": "mp-1"})

    assert response.status_code == 201
    assert response.mimetype == "application/json"
    body = response.get_json()
    assert ObjectId.is_valid(body["_id"])
    assert body["status"] == "installing"
    assert body["ui"] == {"service": "ui", "sdk": "^1.0.0"}


def test_post_unknown_marketplace_addon_is_404(client, addons_collection, monkeypatch):
    monkeypatch.setattr(addons_service.requests, "get", lambda *a, **kw: _marketplace_response(404))

    response = client.post("/api/v1/addons/", json={"marketplace_id": "missing"})

    assert response.status_code == 404
    assert addons_collection.docs == []


@pytest.mark.parametrize(
    "marketplace_get",
    [
        MagicMock(side_effect=requests.ConnectionError("refused")),
        MagicMock(side_effect=requests.Timeout("slow")),
        MagicMock(return_value=_marketplace_response(500)),
    ],
    ids=["connection-error", "timeout", "server-error"],
)
def test_post_with_marketplace_unavailable_is_502(
    client, addons_collection, monkeypatch, marketplace_get
):
    monkeypatch.setattr(addons_service.requests, "get", marketplace_get)

    response = client.post("/api/v1/addons/", json={"marketplace_id": "mp-1"})

    assert response.status_code == 502
    assert addons_collection.docs == []


@pytest.mark.parametrize(
    "existing_status, expected_code",
    [("active", 409), ("installing", 409), ("disabled", 201)],
)
def test_post_ui_addon_with_taken_name(
    client, addons_collection, monkeypatch, existing_status, expected_code
):
    addons_collection.docs.append(
        {"name": "metrics", "status": existing_status, "ui": {"service": "ui", "sdk": "^1.0.0"}}
    )
    marketplace_addon = {
        "name": "metrics",
        "services": [{"service_name": "ui", "image": "ui:2.0"}],
        "ui": {"service": "ui", "sdk": "^1.0.0"},
    }
    monkeypatch.setattr(
        addons_service.requests,
        "get",
        lambda *a, **kw: _marketplace_response(200, marketplace_addon),
    )

    response = client.post("/api/v1/addons/", json={"marketplace_id": "mp-2"})

    assert response.status_code == expected_code


def test_post_addon_without_ui_ignores_name_clash(client, addons_collection, monkeypatch):
    addons_collection.docs.append(
        {"name": "metrics", "status": "active", "ui": {"service": "ui", "sdk": "^1.0.0"}}
    )
    marketplace_addon = {"name": "metrics", "services": [{"service_name": "x", "image": "x"}]}
    monkeypatch.setattr(
        addons_service.requests,
        "get",
        lambda *a, **kw: _marketplace_response(200, marketplace_addon),
    )

    response = client.post("/api/v1/addons/", json={"marketplace_id": "mp-3"})

    assert response.status_code == 201
