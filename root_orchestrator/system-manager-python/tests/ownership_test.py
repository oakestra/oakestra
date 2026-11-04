from unittest.mock import patch

import pytest
from services import ownership
from services.ownership import app_owner, service_owner

APP = {"_id": "app1", "applicationID": "app1", "userId": "alice"}
SERVICE = {"_id": "svc1", "applicationID": "app1"}


@pytest.fixture
def backend():
    with (
        patch.object(ownership, "app_operations") as apps,
        patch.object(ownership, "job_operations") as jobs,
    ):
        apps.get_app.side_effect = lambda appid: APP if appid == "app1" else None
        jobs.get_job_by_id.side_effect = lambda sid: SERVICE if sid == "svc1" else None
        yield apps, jobs


def as_roles(*roles):
    return patch.object(ownership, "get_jwt", return_value={"roles": list(roles)})


def test_admin_acts_as_application_owner(backend):
    with as_roles("Admin"):
        assert app_owner("app1", "Admin") == "alice"
        assert service_owner("svc1", "Admin") == "alice"


def test_regular_user_always_acts_as_self(backend):
    apps, jobs = backend
    with as_roles("Application_Provider"):
        assert app_owner("app1", "bob") == "bob"
        assert service_owner("svc1", "bob") == "bob"
    apps.get_app.assert_not_called()
    jobs.get_job_by_id.assert_not_called()


def test_admin_unknown_ids_fall_back_to_caller(backend):
    with as_roles("Admin"):
        assert app_owner("missing", "Admin") == "Admin"
        assert app_owner(None, "Admin") == "Admin"
        assert service_owner("missing", "Admin") == "Admin"


def test_no_jwt_context_is_not_admin():
    with patch.object(ownership, "get_jwt", side_effect=RuntimeError("no request")):
        assert ownership.caller_is_admin() is False
