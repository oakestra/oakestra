from unittest.mock import MagicMock

import mongomock
import pymongo
import pytest
from ext_requests import mongodb_client, organization_db, user_db
from users import user_management
from werkzeug.security import check_password_hash


def mockdb():
    client = pymongo.MongoClient("mongodb://localhost:10007/users")
    client.drop_database("users")
    mongodb_client.mongo_users = client.db["user"]
    mongodb_client.mongo_organization = client.db["organization"]
    mongodb_client.app = MagicMock()


def root_orga_id():
    return str(organization_db.mongo_get_organization_by_name("root")["_id"])


@pytest.mark.parametrize("password", ["plaintext", "", None])
@mongomock.patch(servers=(("localhost", 10007),))
def test_update_does_not_touch_password(password):
    mockdb()
    user_db.create_admin()

    user_management.user_add(
        "Admin", {"email": "admin@example.com", "password": password}, root_orga_id()
    )

    admin = user_db.mongo_get_user_by_name("Admin")
    assert check_password_hash(admin["password"], "Admin")
    assert admin["email"] == "admin@example.com"


@mongomock.patch(servers=(("localhost", 10007),))
def test_update_still_sets_roles():
    mockdb()
    user_db.create_admin()
    orga_id = root_orga_id()

    user_management.user_add("Admin", {"roles": ["Admin"]}, orga_id)

    admin = user_db.mongo_get_user_by_name("Admin", orga_id)
    assert admin["roles"] == ["Admin"]
