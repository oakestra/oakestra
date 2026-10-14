"""Shared pytest setup for the system-manager test suite.

``services/service_management.py`` imports the network-plugin helpers by name
(``from ext_requests.net_plugin_requests import net_inform_service_deploy, ...``), so the
stub has to be registered in ``sys.modules`` *before* anything imports that package.
pytest imports ``conftest.py`` ahead of every test module, which makes this independent of
collection order. Doing it inside a test module instead only works if that module happens
to be imported first.
"""

import sys
from unittest.mock import MagicMock

net_plugin_stub = MagicMock()
sys.modules["ext_requests.net_plugin_requests"] = net_plugin_stub
