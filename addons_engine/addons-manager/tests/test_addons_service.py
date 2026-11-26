from datetime import timezone
from unittest.mock import MagicMock

from services import addons_service


def test_install_addon_copies_ui_and_sets_timestamps(addons_collection, monkeypatch):
    marketplace_addon = {
        "name": "metrics",
        "description": "Dashboards",
        "services": [{"service_name": "ui", "image": "ui:1.0"}],
        "ui": {"service": "ui", "sdk": "^1.0.0"},
    }
    response = MagicMock(status_code=200)
    response.json.return_value = marketplace_addon
    monkeypatch.setattr(addons_service.requests, "get", lambda *a, **kw: response)

    addon = addons_service.install_addon({"marketplace_id": "mp-1"})

    assert addon["ui"] == {"service": "ui", "sdk": "^1.0.0"}
    assert addon["status"] == "installing"
    assert addon["created_at"].tzinfo == timezone.utc
    assert addon["created_at"] == addon["updated_at"]


def test_install_addon_without_ui_stores_none(addons_collection, monkeypatch):
    response = MagicMock(status_code=200)
    response.json.return_value = {"name": "plain", "services": [{"service_name": "a"}]}
    monkeypatch.setattr(addons_service.requests, "get", lambda *a, **kw: response)

    addon = addons_service.install_addon({"marketplace_id": "mp-2"})

    assert "ui" in addon
    assert addon["ui"] is None
