import copy
import importlib
import sys
import unittest
from unittest.mock import ANY, MagicMock

import mongomock as mongomock
import pymongo
from ext_requests import apps_db, mongodb_client
from tests.utils import get_full_random_sla_app

sys.modules["ext_requests.net_plugin_requests"] = unittest.mock.Mock()
net_plugin = sys.modules["ext_requests.net_plugin_requests"]
net_plugin.net_inform_service_deploy = MagicMock()

# Note:
# We need to run the code above before importing the following, otherwise errors occur.
# We could simply use normal imports and it will work, but this violates the PEP8 code style.
sm = importlib.import_module("services.service_management")
create_services_of_app = sm.create_services_of_app
delete_service = sm.delete_service
generate_db_structure = sm.generate_db_structure
get_all_services = sm.get_all_services
get_service = sm.get_service
update_service = sm.update_service
user_services = sm.user_services


@mongomock.patch(servers=(("localhost", 10007),))
def test_create_service_with_app():
    # SETUP
    mockdb()

    sla = get_full_random_sla_app()
    db_app_mock = copy.deepcopy(sla)

    db_app_mock["applications"][0]["userId"] = "Admin"
    db_app_mock["applications"][0]["microservices"] = []
    app_id = apps_db.mongo_add_application(db_app_mock["applications"][0])
    sla["applications"][0]["applicationID"] = app_id

    microservices = sla["applications"][0]["microservices"]
    last_service = microservices[len(microservices) - 1]
    last_service = generate_db_structure(db_app_mock["applications"][0], last_service)

    # EXEC
    result, code = create_services_of_app("Admin", sla)

    # ASSERT
    assert code == 200
    db_app_result = apps_db.mongo_find_app_by_name_and_namespace(
        sla["applications"][0]["application_name"],
        sla["applications"][0]["application_namespace"],
    )
    net_plugin.net_inform_service_deploy.assert_called_with(last_service, ANY)
    assert len(db_app_result["microservices"]) == len(sla["applications"][0]["microservices"])


@mongomock.patch(servers=(("localhost", 10007),))
def test_create_service_without_app():
    # SETUP
    mockdb()
    sla = get_full_random_sla_app()
    sla["applications"][0]["applicationID"] = "63219606def3818062c12cd3"

    # EXEC
    result, code = create_services_of_app("Admin", sla)

    # ASSERT
    assert code == 404
    db_app_result = apps_db.mongo_find_app_by_name_and_namespace(
        sla["applications"][0]["application_name"],
        sla["applications"][0]["application_namespace"],
    )
    assert db_app_result is None


@mongomock.patch(servers=(("localhost", 10007),))
def test_create_invalid_service_name():
    # SETUP
    mockdb()

    sla = get_full_random_sla_app()
    db_app_mock = copy.deepcopy(sla)

    db_app_mock["applications"][0]["userId"] = "Admin"
    db_app_mock["applications"][0]["microservices"] = []
    app_id = apps_db.mongo_add_application(db_app_mock["applications"][0])
    sla["applications"][0]["applicationID"] = app_id

    sla["applications"][0]["microservices"][0]["microservice_name"] = "TOOLONGNAME"

    # EXEC
    result, code = create_services_of_app("Admin", sla)

    # ASSERT
    assert code == 403


@mongomock.patch(servers=(("localhost", 10007),))
def test_create_invalid_service_namespace():
    # SETUP
    mockdb()

    sla = get_full_random_sla_app()
    db_app_mock = copy.deepcopy(sla)

    db_app_mock["applications"][0]["userId"] = "Admin"
    db_app_mock["applications"][0]["microservices"] = []
    app_id = apps_db.mongo_add_application(db_app_mock["applications"][0])
    sla["applications"][0]["applicationID"] = app_id

    sla["applications"][0]["microservices"][0][
        "microservice_namespace"
    ] = "THISNAMESPACEISTOOLONGTOBECCEPTED"

    # EXEC
    result, code = create_services_of_app("Admin", sla)

    # ASSERT
    assert code == 403


@mongomock.patch(servers=(("localhost", 10007),))
def test_delete_service():
    # SETUP
    mockdb()

    sla = get_full_random_sla_app()
    db_app_mock = copy.deepcopy(sla)

    db_app_mock["applications"][0]["userId"] = "Admin"
    db_app_mock["applications"][0]["microservices"] = []
    app_id = apps_db.mongo_add_application(db_app_mock["applications"][0])
    sla["applications"][0]["applicationID"] = app_id

    result, code = create_services_of_app("Admin", sla)
    db_app_before_deletion = apps_db.mongo_find_app_by_name_and_namespace(
        sla["applications"][0]["application_name"],
        sla["applications"][0]["application_namespace"],
    )
    service_to_be_deleted = db_app_before_deletion["microservices"][0]

    # EXEC
    result = delete_service("Admin", service_to_be_deleted)
    resultNone = delete_service("Admin", service_to_be_deleted)

    # ASSERT
    assert result
    assert not resultNone
    db_app_after_deletion = apps_db.mongo_find_app_by_name_and_namespace(
        sla["applications"][0]["application_name"],
        sla["applications"][0]["application_namespace"],
    )
    assert service_to_be_deleted not in db_app_after_deletion["microservices"]
    assert apps_db.mongo_find_job_by_id(service_to_be_deleted) is None


@mongomock.patch(servers=(("localhost", 10007),))
def test_update_service():
    # SETUP
    mockdb()

    sla = get_full_random_sla_app()
    db_app_mock = copy.deepcopy(sla)

    db_app_mock["applications"][0]["userId"] = "Admin"
    db_app_mock["applications"][0]["microservices"] = []
    app_id = apps_db.mongo_add_application(db_app_mock["applications"][0])
    sla["applications"][0]["applicationID"] = app_id

    result, code = create_services_of_app("Admin", sla)
    db_app_before_deletion = apps_db.mongo_find_app_by_name_and_namespace(
        sla["applications"][0]["application_name"],
        sla["applications"][0]["application_namespace"],
    )
    service_to_be_updated_id = db_app_before_deletion["microservices"][0]
    service_to_be_updated = apps_db.mongo_find_job_by_id(service_to_be_updated_id)
    service_to_be_updated["new_field"] = "new"

    # EXEC
    result, code = update_service("Admin", service_to_be_updated, service_to_be_updated_id)

    # ASSERT
    assert code == 200
    db_app_after_update = apps_db.mongo_find_job_by_id(service_to_be_updated_id)
    assert db_app_after_update["new_field"] == "new"


@mongomock.patch(servers=(("localhost", 10007),))
def test_update_service_not_found():
    # SETUP
    mockdb()

    sla = get_full_random_sla_app()
    db_app_mock = copy.deepcopy(sla)

    db_app_mock["applications"][0]["userId"] = "Admin"
    db_app_mock["applications"][0]["microservices"] = []
    app_id = apps_db.mongo_add_application(db_app_mock["applications"][0])
    sla["applications"][0]["applicationID"] = app_id

    result, code = create_services_of_app("Admin", sla)
    db_app_before_deletion = apps_db.mongo_find_app_by_name_and_namespace(
        sla["applications"][0]["application_name"],
        sla["applications"][0]["application_namespace"],
    )
    service_to_be_updated_id = db_app_before_deletion["microservices"][0]
    service_to_be_updated = apps_db.mongo_find_job_by_id(service_to_be_updated_id)
    service_to_be_updated["new_field"] = "new"
    delete_service("Admin", service_to_be_updated_id)

    # EXEC
    result, code = update_service("Admin", service_to_be_updated, service_to_be_updated_id)

    # ASSERT
    assert code == 404


@mongomock.patch(servers=(("localhost", 10007),))
def test_get_user_services():
    # SETUP
    mockdb()

    sla = get_full_random_sla_app()
    db_app_mock = copy.deepcopy(sla)

    db_app_mock["applications"][0]["userId"] = "Admin"
    db_app_mock["applications"][0]["microservices"] = []
    app_id = apps_db.mongo_add_application(db_app_mock["applications"][0])
    sla["applications"][0]["applicationID"] = app_id

    result, code = create_services_of_app("Admin", sla)
    apps_db.mongo_find_app_by_name_and_namespace(
        sla["applications"][0]["application_name"],
        sla["applications"][0]["application_namespace"],
    )

    # EXEC
    result, code = user_services(app_id, "Admin")
    resultNone, codeNone = user_services(app_id, "AdminFake")

    # ASSERT
    assert code == 200
    assert codeNone == 404
    assert list(result) == list(apps_db.mongo_get_jobs_of_application(app_id))
    assert resultNone == {"message": "app not found"}


@mongomock.patch(servers=(("localhost", 10007),))
def test_get_services():
    # SETUP
    mockdb()

    sla = get_full_random_sla_app()
    db_app_mock = copy.deepcopy(sla)

    db_app_mock["applications"][0]["userId"] = "Admin"
    db_app_mock["applications"][0]["microservices"] = []
    app_id = apps_db.mongo_add_application(db_app_mock["applications"][0])
    sla["applications"][0]["applicationID"] = app_id
    result, code = create_services_of_app("Admin", sla)

    # EXEC
    services = get_all_services()
    for service in list(services):
        service_result = get_service(service["microserviceID"], "Admin")
        # ASSERT
        assert service_result == apps_db.mongo_find_job_by_id(service["microserviceID"])


def mockdb():
    mongodb_client.mongo_jobs = pymongo.MongoClient("mongodb://localhost:10007/jobs")
    mongodb_client.mongo_clusters = pymongo.MongoClient("mongodb://localhost:10007/clusters")
    mongodb_client.mongo_users = pymongo.MongoClient("mongodb://localhost:10007/users").db["user"]
    mongodb_client.mongo_applications = mongodb_client.mongo_jobs.db["apps"]
    mongodb_client.mongo_services = mongodb_client.mongo_jobs.db["jobs"]
    mongodb_client.app = MagicMock()
