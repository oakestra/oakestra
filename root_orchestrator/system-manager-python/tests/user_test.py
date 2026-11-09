from datetime import datetime
from unittest.mock import MagicMock

import mongomock
import pymongo
from ext_requests import mongodb_client, organization_db, user_db
from users import user_management
from werkzeug.security import check_password_hash


def mockdb():
    client = pymongo.MongoClient("mongodb://localhost:10007/users")
    client.drop_database("users")
    mongodb_client.mongo_users = client.db["user"]
    mongodb_client.mongo_organization = client.db["organization"]
    mongodb_client.app = MagicMock()


def setup_root_with_user():
    user_db.create_admin()
    orga_id = str(organization_db.mongo_get_organization_by_name("root")["_id"])
    user = user_db.mongo_save_user(
        {
            "name": "alice",
            "email": "alice@example.com",
            "password": "some-hash",
            "created_at": "01/01/2026 00:00",
            "roles": ["Application_Provider"],
        },
        orga_id,
    )
    return orga_id, user


@mongomock.patch(servers=(("localhost", 10007),))
def test_register_response_has_no_password():
    mockdb()
    _, user = setup_root_with_user()

    assert user["name"] == "alice"
    assert user["roles"] == ["Application_Provider"]
    assert "password" not in user


@mongomock.patch(servers=(("localhost", 10007),))
def test_get_all_users_has_no_passwords_or_reset_tokens():
    mockdb()
    setup_root_with_user()
    admin = user_db.mongo_get_user_by_name("Admin")
    user_db.mongo_create_password_reset_token(str(admin["_id"]), datetime.now(), "token-hash")

    users = user_management.user_get_all()

    assert sorted(u["name"] for u in users) == ["Admin", "alice"]
    assert all("password" not in u for u in users)


@mongomock.patch(servers=(("localhost", 10007),))
def test_get_organization_users_has_no_passwords():
    mockdb()
    orga_id, _ = setup_root_with_user()

    users = user_management.user_get_all_from_Organization(orga_id)

    assert sorted(u["name"] for u in users) == ["Admin", "alice"]
    assert all("password" not in u for u in users)


@mongomock.patch(servers=(("localhost", 10007),))
def test_get_single_user_has_no_password():
    mockdb()
    orga_id, _ = setup_root_with_user()

    user = user_management.user_get_by_name("alice", orga_id)

    assert user["roles"] == ["Application_Provider"]
    assert "password" not in user


@mongomock.patch(servers=(("localhost", 10007),))
def test_delete_user_response_has_no_passwords():
    mockdb()
    setup_root_with_user()

    users = user_management.user_delete("alice")

    assert [u["name"] for u in users] == ["Admin"]
    assert "password" not in users[0]


@mongomock.patch(servers=(("localhost", 10007),))
def test_login_lookup_still_sees_password_hash():
    mockdb()
    user_db.create_admin()

    admin = user_db.mongo_get_user_by_name("Admin")

    assert check_password_hash(admin["password"], "Admin")
