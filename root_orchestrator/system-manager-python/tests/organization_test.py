import copy
import sys
import unittest
from unittest.mock import MagicMock

import mongomock as mongomock
import pymongo
from ext_requests import mongodb_client, organization_db, user_db

sys.modules["ext_requests.net_plugin_requests"] = unittest.mock.Mock()
net_plugin = sys.modules["ext_requests.net_plugin_requests"]
net_plugin.net_inform_service_deploy = MagicMock()


@mongomock.patch(servers=(("localhost", 10007),))
def test_add_organization():
    # SETUP
    mockdb()

    orga = {
        "name": "root",
        "member": [
            {
                "user_id": "6427d9f43c9f60e47cf93dc9",
                "roles": [
                    "Admin",
                ],
            }
        ],
    }

    # EXEC
    copy.deepcopy(orga)
    organization_db.mongo_add_organization(orga)

    # ASSERT
    assert organization_db.mongo_get_organization_by_name("root") is not None


@mongomock.patch(servers=(("localhost", 10007),))
def test_add_user_to_orga():
    # SETUP
    mockdb()

    user_db.create_admin()
    user = user_db.mongo_get_user_by_name("Admin")
    orga = organization_db.mongo_get_organization_by_name("root")

    user_id = str(user["_id"])
    orga_id = str(orga["_id"])

    # EXEC
    organization_db.mongo_add_user_role_to_organization(user_id, orga_id, ["Admin"])

    # ASSERT
    roles = organization_db.mongo_get_roles_of_user_in_organization(user_id, orga_id)
    assert roles == ["Admin"]


@mongomock.patch(servers=(("localhost", 10007),))
def test_get_organization():
    # SETUP
    mockdb()

    user_db.create_admin()
    orga = organization_db.mongo_get_organization_by_name("root")

    organizations = organization_db.mongo_get_all_organizations()
    for o in list(organizations):
        # ASSERT
        assert o == orga


@mongomock.patch(servers=(("localhost", 10007),))
def test_update_organization():
    # SETUP
    mockdb()

    user_db.create_admin()
    orga = organization_db.mongo_get_organization_by_name("root")
    orga_id = str(orga["_id"])
    orga["name"] = "Test"

    # EXEC
    newOrga = organization_db.mongo_update_organizations(orga_id, orga)

    # ASSERT
    assert newOrga["name"] == "Test"


@mongomock.patch(servers=(("localhost", 10007),))
def test_delete_role_entrys():
    # SETUP
    mockdb()

    user_db.create_admin()
    user = user_db.mongo_get_user_by_name("Admin")
    user_id = str(user["_id"])

    # EXEC
    organization_db.mongo_delete_all_role_entrys_of_user(user_id)
    orga = organization_db.mongo_get_organization_by_name("root")

    # ASSERT
    assert orga["member"] == []


def mockdb():
    mongodb_client.mongo_jobs = pymongo.MongoClient("mongodb://localhost:10007/jobs")
    mongodb_client.mongo_clusters = pymongo.MongoClient("mongodb://localhost:10007/clusters")
    mongodb_client.mongo_users = pymongo.MongoClient("mongodb://localhost:10007/users").db["user"]
    mongodb_client.mongo_organization = pymongo.MongoClient("mongodb://localhost:10007/users").db[
        "organization"
    ]
    mongodb_client.mongo_applications = mongodb_client.mongo_jobs.db["apps"]
    mongodb_client.mongo_services = mongodb_client.mongo_jobs.db["jobs"]
    mongodb_client.app = MagicMock()
