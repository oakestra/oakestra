from unittest.mock import patch

import pytest

from ..services import service_management


@pytest.fixture
def net_deploy_mock():
    with patch.object(service_management, "net_inform_service_deploy") as mock:
        yield mock


@pytest.fixture
def net_undeploy_mock():
    with patch.object(service_management, "net_inform_service_undeploy") as mock:
        yield mock
