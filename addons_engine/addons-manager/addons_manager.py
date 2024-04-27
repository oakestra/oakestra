import logging
import os
import signal
import socket
import sys
import uuid

from api.v1.addons_api import addonsblp, init_addons_socket
from db.mongodb_client import mongo_init
from flask import Flask
from flask_smorest import Api
from flask_socketio import SocketIO
from flask_swagger_ui import get_swaggerui_blueprint
from services.addons_runner import init_addon_manager
from services.cleanup_handler import handle_shutdown

ADDONS_MANAGER_PORT = os.environ.get("ADDONS_MANAGER_PORT") or 11101
ADDONS_MANAGER_ID = os.environ.get("ADDONS_MANAGER_ID") or str(uuid.uuid4())

app = Flask(__name__)

app.config["OPENAPI_VERSION"] = "3.0.2"
app.config["API_TITLE"] = "Addon Manager Api"
app.config["API_VERSION"] = "v1"
app.config["OPENAPI_URL_PREFIX"] = "/docs"


mongo_init(app)
init_addon_manager(ADDONS_MANAGER_ID)

socketio = SocketIO(app)
init_addons_socket(socketio, ADDONS_MANAGER_ID)

api = Api(app)

# Register blueprints
SWAGGER_URL = "/api/docs"
API_URL = "/docs/openapi.json"
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={"app_name": "Addon Manager"},
)
app.register_blueprint(swaggerui_blueprint)

api.register_blueprint(addonsblp)


def signal_handler(sig, frame):
    logging.info("Shutting down Addon Manager...")
    handle_shutdown()

    sys.exit(0)


@app.route("/", methods=["GET"])
def health():
    return "ok"


if __name__ == "__main__":
    import eventlet

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    eventlet.wsgi.server(
        eventlet.listen(("::", int(ADDONS_MANAGER_PORT)), family=socket.AF_INET6), app
    )
