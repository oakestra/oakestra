from bson.objectid import ObjectId
from db import addons_db


def test_find_active_addons_filters_on_active_status(addons_collection):
    addons_collection.docs.extend(
        [{"name": "a", "status": "active"}, {"name": "b", "status": "enabled"}]
    )

    assert [addon["name"] for addon in addons_db.find_active_addons()] == ["a"]
    assert addons_collection.queries[-1] == {"status": "active"}


def test_find_active_addons_with_ui_requires_ui_block(addons_collection):
    addons_collection.docs.extend(
        [
            {"name": "a", "status": "active", "ui": {"service": "ui", "sdk": "^1"}},
            {"name": "b", "status": "active", "ui": None},
        ]
    )

    assert [addon["name"] for addon in addons_db.find_active_addons(with_ui=True)] == ["a"]
    assert addons_collection.queries[-1] == {"status": "active", "ui": {"$ne": None}}


def test_invalid_ids_behave_like_not_found(addons_collection):
    assert addons_db.find_addon_by_id("nope") is None
    assert addons_db.update_addon("nope", {"status": "active"}) is None
    assert addons_db.delete_addon_by_id("nope") is None


def test_update_addon_sets_updated_at_without_mutating_input(addons_collection):
    addon_id = ObjectId()
    addons_collection.docs.append({"_id": addon_id, "status": "installing"})
    update = {"status": "active"}

    result = addons_db.update_addon(str(addon_id), update)

    assert result["status"] == "active"
    assert result["updated_at"] is not None
    assert update == {"status": "active"}
