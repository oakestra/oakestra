import os

from api.v1.addons_blueprint import addonsblp
from db.mongodb_client import mongo_init
from flask import Flask
from flask_smorest import Api
from flask_swagger_ui import get_swaggerui_blueprint
from services.addons_service import init_container_manager

ADDON_ENGINE_PORT = os.environ.get("ADDON_ENGINE_PORT") or 11101

app = Flask(__name__)

app.config["OPENAPI_VERSION"] = "3.0.2"
app.config["API_TITLE"] = "Addon Engine Api"
app.config["API_VERSION"] = "v1"
app.config["OPENAPI_URL_PREFIX"] = "/docs"

mongo_init(app)

init_container_manager()

api = Api(app)

# Register blueprints
SWAGGER_URL = "/api/docs"
API_URL = "/docs/openapi.json"
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={"app_name": "Addon Engine"},
)
app.register_blueprint(swaggerui_blueprint)

api.register_blueprint(addonsblp)


@app.route("/", methods=["GET"])
def health():
    return "ok"


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=ADDON_ENGINE_PORT, debug=False)
