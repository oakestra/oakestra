import pytest


def _addon(name, status="active", ui_service="ui", image="ghcr.io/x/ui:0.1.0", sdk="^1.0.0"):
    return {
        "name": name,
        "status": status,
        "services": [
            {"service_name": "backend", "image": "ghcr.io/x/backend:9.9.9"},
            {"service_name": "ui", "image": image},
        ],
        "ui": {"service": ui_service, "sdk": sdk} if ui_service else None,
    }


@pytest.mark.parametrize("path", ["/api/v1/ui-addons", "/api/v1/ui-addons/"])
def test_serves_json_with_and_without_trailing_slash(client, addons_collection, path):
    addons_collection.docs.append(_addon("metrics"))

    response = client.get(path)

    assert response.status_code == 200
    assert response.mimetype == "application/json"
    assert response.get_json() == [{"name": "metrics", "version": "0.1.0", "sdk": "^1.0.0"}]


def test_lists_only_active_addons_with_ui(client, addons_collection):
    addons_collection.docs.extend(
        [
            _addon("active-ui"),
            _addon("installing-ui", status="installing"),
            _addon("partial-ui", status="partially_active"),
            _addon("no-ui", ui_service=None),
        ]
    )
    # Installed before install_addon copied ui, so there is no ui key at all.
    legacy = _addon("legacy")
    del legacy["ui"]
    addons_collection.docs.append(legacy)

    response = client.get("/api/v1/ui-addons")

    assert [entry["name"] for entry in response.get_json()] == ["active-ui"]


def test_keeps_first_entry_for_duplicate_names(client, addons_collection):
    addons_collection.docs.extend(
        [_addon("dup", image="ui:1.0"), _addon("dup", image="ui:2.0"), _addon("other")]
    )

    response = client.get("/api/v1/ui-addons")

    assert response.get_json() == [
        {"name": "dup", "version": "1.0", "sdk": "^1.0.0"},
        {"name": "other", "version": "0.1.0", "sdk": "^1.0.0"},
    ]


def test_skips_addon_whose_ui_service_is_missing(client, addons_collection):
    addons_collection.docs.extend([_addon("broken", ui_service="frontend"), _addon("fine")])

    response = client.get("/api/v1/ui-addons")

    assert [entry["name"] for entry in response.get_json()] == ["fine"]


def test_skips_addon_with_invalid_name(client, addons_collection):
    addons_collection.docs.extend([_addon("Not Valid"), _addon("valid-1")])

    response = client.get("/api/v1/ui-addons")

    assert [entry["name"] for entry in response.get_json()] == ["valid-1"]


def test_empty_list_when_nothing_qualifies(client, addons_collection):
    response = client.get("/api/v1/ui-addons")

    assert response.status_code == 200
    assert response.get_json() == []


@pytest.mark.parametrize("sdk", [None, ""])
def test_skips_addon_without_sdk(client, addons_collection, sdk):
    addons_collection.docs.extend([_addon("no-sdk", sdk=sdk), _addon("fine")])

    response = client.get("/api/v1/ui-addons")

    assert [entry["name"] for entry in response.get_json()] == ["fine"]
