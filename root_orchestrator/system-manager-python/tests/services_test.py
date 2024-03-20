import copy
import os
import sys
from unittest.mock import ANY, MagicMock, Mock, patch

import pytest
from resource_abstractor_client import app_operations, job_operations
from testcontainers.compose import DockerCompose
from tests.utils import get_first_app, get_full_random_sla_app

sys.modules["ext_requests.net_plugin_requests"] = Mock()
net_plugin = sys.modules["ext_requests.net_plugin_requests"]
net_plugin.net_inform_service_deploy = MagicMock()

# we ignore E402 because we need to import the service_management module after the mock
from services.service_management import (  # noqa: E402
    create_services_of_app,
    delete_service,
    generate_db_structure,
    get_all_services,
    get_service,
    update_service,
    user_services,
)

current_dir = os.path.dirname(os.path.abspath(__file__))


@pytest.fixture(scope="session")
def resource_abstractor():
    with DockerCompose(current_dir) as compose:
        service_uri = "http://localhost:21011"
        compose.wait_for(service_uri)
        yield service_uri


def test_create_service_with_app(resource_abstractor):
    with patch(
        "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
        new=str(resource_abstractor),
    ):
        sla = get_full_random_sla_app()
        sla_first_app = get_first_app(sla)
        app_mock = copy.deepcopy(sla_first_app)

        app_mock["userId"] = "Admin"
        app_mock["microservices"] = []

        created_app = app_operations.create_app("Admin", app_mock)
        sla_first_app["applicationID"] = created_app["applicationID"]

        microservices = sla_first_app["microservices"]
        last_service = microservices[-1]
        last_service = generate_db_structure(created_app, last_service)

        # EXEC
        result, code = create_services_of_app("Admin", sla)

        # ASSERT
        assert code == 200
        result = app_operations.get_app_by_name_and_namespace(
            sla_first_app["application_name"],
            sla_first_app["application_namespace"],
            "Admin",
        )
        net_plugin.net_inform_service_deploy.assert_called_with(last_service, ANY)
        assert len(result["microservices"]) == len(sla_first_app["microservices"])


def test_create_service_without_app(resource_abstractor):
    with patch(
        "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
        new=str(resource_abstractor),
    ):
        # SETUP
        sla = get_full_random_sla_app()
        sla_first_app = get_first_app(sla)
        sla_first_app["applicationID"] = "63219606def3818062c12cd3"

        # EXEC
        result, code = create_services_of_app("Admin", sla)

        # ASSERT
        assert code == 404
        app_result = app_operations.get_app_by_name_and_namespace(
            sla_first_app["application_name"],
            sla_first_app["application_namespace"],
            "Admin",
        )
        assert app_result is None


def test_create_invalid_service_name(resource_abstractor):
    with patch(
        "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
        new=str(resource_abstractor),
    ):
        sla = get_full_random_sla_app()
        sla_first_app = get_first_app(sla)
        app_mock = copy.deepcopy(sla_first_app)

        app_mock["userId"] = "Admin"
        app_mock["microservices"] = []

        app = app_operations.create_app("Admin", app_mock)

        sla_first_app["applicationID"] = app["applicationID"]
        sla_first_app["microservices"][0]["microservice_name"] = "TOOLONGNAME"

        # EXEC
        result, code = create_services_of_app("Admin", sla)

        # ASSERT
        assert code == 403


def test_create_invalid_service_namespace(resource_abstractor):
    with patch(
        "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
        new=str(resource_abstractor),
    ):
        sla = get_full_random_sla_app()
        sla_first_app = get_first_app(sla)
        app_mock = copy.deepcopy(sla_first_app)

        app_mock["userId"] = "Admin"
        app_mock["microservices"] = []

        app = app_operations.create_app("Admin", app_mock)
        sla_first_app["applicationID"] = app["applicationID"]

        sla_first_app["microservices"][0][
            "microservice_namespace"
        ] = "THISNAMESPACEISTOOLONGTOBECCEPTED"

        # EXEC
        result, code = create_services_of_app("Admin", sla)

        # ASSERT
        assert code == 403


def test_delete_service(resource_abstractor):
    with patch(
        "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
        new=str(resource_abstractor),
    ):
        sla = get_full_random_sla_app()
        sla_first_app = get_first_app(sla)
        app_mock = copy.deepcopy(sla_first_app)

        app_mock["userId"] = "Admin"
        app_mock["microservices"] = []

        app = app_operations.create_app("Admin", app_mock)
        sla_first_app["applicationID"] = app["applicationID"]

        result, code = create_services_of_app("Admin", sla)

        app_before_deletion = app_operations.get_app_by_name_and_namespace(
            sla_first_app["application_name"],
            sla_first_app["application_namespace"],
            "Admin",
        )
        service_to_be_deleted = app_before_deletion["microservices"][0]

        # EXEC
        result = delete_service("Admin", service_to_be_deleted)
        resultNone = delete_service("Admin", service_to_be_deleted)

        # ASSERT
        assert result
        assert not resultNone
        db_app_after_deletion = app_operations.get_app_by_name_and_namespace(
            sla_first_app["application_name"],
            sla_first_app["application_namespace"],
            "Admin",
        )
        assert service_to_be_deleted not in db_app_after_deletion["microservices"]
        assert job_operations.get_job_by_id(service_to_be_deleted) is None


# TODO Commented it out until a proper update)service is implemented
# def test_update_service(resource_abstractor):
#     with patch(
#         "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
#         new=str(resource_abstractor),
#     ):
#         sla = get_full_random_sla_app()
#         sla_first_app = get_first_app(sla)
#         app_mock = copy.deepcopy(sla_first_app)

#         app_mock["userId"] = "Admin"
#         app_mock["microservices"] = []

#         app = app_operations.create_app("Admin", app_mock)
#         sla_first_app["applicationID"] = app["applicationID"]

#         result, code = create_services_of_app("Admin", sla)
#         app_before_deletion = app_operations.get_app_by_name_and_namespace(
#             sla_first_app["application_name"],
#             sla_first_app["application_namespace"],
#             "Admin",
#         )

#         service_to_be_updated_id = app_before_deletion["microservices"][0]
#         service_to_be_updated = job_operations.get_job_by_id(service_to_be_updated_id)
#         service_to_be_updated["new_field"] = "new"

#         # EXEC
#         result, code = update_service("Admin", service_to_be_updated, service_to_be_updated_id)

#         # ASSERT
#         assert code == 200
#         app_after_update = job_operations.get_job_by_id(service_to_be_updated_id)
#         assert app_after_update["new_field"] == "new"


def test_update_service_not_found(resource_abstractor):
    with patch(
        "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
        new=str(resource_abstractor),
    ):
        sla = get_full_random_sla_app()
        sla_first_app = get_first_app(sla)
        app_mock = copy.deepcopy(sla_first_app)

        app_mock["userId"] = "Admin"
        app_mock["microservices"] = []

        app = app_operations.create_app("Admin", app_mock)
        sla_first_app["applicationID"] = app["applicationID"]

        result, code = create_services_of_app("Admin", sla)
        app_before_deletion = app_operations.get_app_by_name_and_namespace(
            sla_first_app["application_name"],
            sla_first_app["application_namespace"],
            "Admin",
        )
        service_to_be_updated_id = app_before_deletion["microservices"][0]
        service_to_be_updated = job_operations.get_job_by_id(service_to_be_updated_id)
        service_to_be_updated["new_field"] = "new"
        delete_service("Admin", service_to_be_updated_id)

        # EXEC
        result, code = update_service("Admin", service_to_be_updated, service_to_be_updated_id)

        # ASSERT
        assert code == 404


def test_get_user_services(resource_abstractor):
    with patch(
        "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
        new=str(resource_abstractor),
    ):
        sla = get_full_random_sla_app()
        sla_first_app = get_first_app(sla)
        app_mock = copy.deepcopy(sla_first_app)

        app_mock["userId"] = "Admin"
        app_mock["microservices"] = []
        app = app_operations.create_app("Admin", app_mock)
        app_id = app["applicationID"]
        sla["applications"][0]["applicationID"] = app_id

        result, code = create_services_of_app("Admin", sla)
        app_operations.get_app_by_name_and_namespace(
            sla_first_app["application_name"],
            sla_first_app["application_namespace"],
            "Admin",
        )

        # EXEC
        result, code = user_services(app_id, "Admin")
        resultNone, codeNone = user_services(app_id, "AdminFake")

        # ASSERT
        assert code == 200
        assert codeNone == 404
        assert list(result) == list(job_operations.get_jobs_of_application(app_id))
        assert resultNone == {"message": "app not found"}


def test_get_services(resource_abstractor):
    with patch(
        "resource_abstractor_client.client_helper.RESOURCE_ABSTRACTOR_ADDR",
        new=str(resource_abstractor),
    ):
        sla = get_full_random_sla_app()
        sla_first_app = get_first_app(sla)
        app_mock = copy.deepcopy(sla_first_app)

        app_mock["userId"] = "Admin"
        app_mock["microservices"] = []

        app = app_operations.create_app("Admin", app_mock)
        sla_first_app["applicationID"] = app["applicationID"]
        result, code = create_services_of_app("Admin", sla)

        # EXEC
        services, code = get_all_services()
        assert code == 200

        for service in list(services):
            service_result = get_service(service["microserviceID"], "Admin")
            # ASSERT
            assert service_result == job_operations.get_job_by_id(service["microserviceID"])
