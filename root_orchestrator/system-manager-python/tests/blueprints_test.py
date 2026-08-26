import json
import pathlib
from unittest.mock import patch

import pytest
from flask import Flask
from flask_smorest import Api

TEST_SLAS_PATH = pathlib.Path.cwd() / "tests" / "service_level_agreements"


@pytest.fixture
def client():
    # Imported here rather than at module level so that this file, which pytest
    # collects before services_test.py, does not become the first importer of
    # services.service_management and bind it to the wrong net_plugin mock.
    from blueprints.applications_blueprints import applicationblp
    from blueprints.services_blueprints import serviceblp

    app = Flask(__name__)
    app.config["API_TITLE"] = "test"
    app.config["API_VERSION"] = "v1"
    app.config["OPENAPI_VERSION"] = "3.0.2"
    api = Api(app)
    api.register_blueprint(serviceblp)
    api.register_blueprint(applicationblp)
    return app.test_client()


@pytest.fixture
def flawed_sla():
    with open(TEST_SLAS_PATH / "sla_flawed_5.json", "r") as f:
        return json.load(f)


def rejected_microservice_name(sla):
    return sla["applications"][0]["microservices"][0]["microservice_name"]


def test_post_service_returns_the_sla_validation_error(client, flawed_sla):
    # /api/service/ is guarded by the repo's own jwt_auth_required wrapper.
    with (
        patch("roles.securityUtils.verify_jwt_in_request"),
        patch("roles.securityUtils.get_jwt", return_value={}),
        patch("blueprints.services_blueprints.get_jwt_auth_identity", return_value="Admin"),
    ):
        response = client.post("/api/service/", json=flawed_sla)

    assert response.status_code == 422
    assert rejected_microservice_name(flawed_sla) in response.get_json()["message"]


def test_post_service_without_a_body_says_so(client):
    with (
        patch("roles.securityUtils.verify_jwt_in_request"),
        patch("roles.securityUtils.get_jwt", return_value={}),
        patch("blueprints.services_blueprints.get_jwt_auth_identity", return_value="Admin"),
    ):
        response = client.post("/api/service/", json={})

    assert response.status_code == 404
    assert "without a yaml file" in response.get_json()["message"]


def test_post_application_returns_the_sla_validation_error(client, flawed_sla):
    # /api/application/ is guarded by flask_jwt_extended's jwt_required directly.
    with (
        patch("flask_jwt_extended.view_decorators.verify_jwt_in_request"),
        patch("blueprints.applications_blueprints.get_jwt_identity", return_value="Admin"),
    ):
        response = client.post("/api/application/", json=flawed_sla)

    assert response.status_code == 422
    assert rejected_microservice_name(flawed_sla) in response.get_json()["message"]
