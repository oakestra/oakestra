import os
import sys

import pytest
from flask import Flask
from flask_smorest import Api

# The service imports its packages as top-level modules (db, services, api).
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from api.v1.marketplace_blueprint import marketplaceblp  # noqa: E402


@pytest.fixture
def client():
    # marketplace_manager.py connects to Mongo at import time, so build a bare app instead.
    app = Flask(__name__)
    app.config["API_TITLE"] = "test"
    app.config["API_VERSION"] = "v1"
    app.config["OPENAPI_VERSION"] = "3.0.2"
    app.url_map.strict_slashes = False
    Api(app).register_blueprint(marketplaceblp)
    return app.test_client()
