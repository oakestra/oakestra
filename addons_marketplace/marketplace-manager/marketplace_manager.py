import os

from api.v1.marketplace_blueprint import marketplaceblp
from db.mongodb_client import mongo_init
from flask import Flask
from flask_cors import CORS
from flask_smorest import Api
from flask_swagger_ui import get_swaggerui_blueprint

ADDON_MARKETPLACE_PORT = int(os.environ.get("ADDON_MARKETPLACE_PORT", 11102))

app = Flask(__name__)

# Configure CORS with explicit settings
CORS(
    app,
    resources={
        r"/api/*": {
            "origins": "*",
            "methods": ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
            "allow_headers": ["Content-Type", "Authorization"],
            "expose_headers": ["Content-Type"],
            "supports_credentials": False,
        }
    },
)

# Disable strict slashes to prevent redirects
app.url_map.strict_slashes = False

app.config["OPENAPI_VERSION"] = "3.0.2"
app.config["API_TITLE"] = "Addon Marketplace Api"
app.config["API_VERSION"] = "v1"
app.config["OPENAPI_URL_PREFIX"] = "/docs"

mongo_init(app)
api = Api(app)


# Register blueprints
SWAGGER_URL = "/api/docs"
API_URL = "/docs/openapi.json"
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={"app_name": "Addon Marketplace"},
)
app.register_blueprint(swaggerui_blueprint)
api.register_blueprint(marketplaceblp)


@app.route("/", methods=["GET"])
def health():
    return "ok"


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=ADDON_MARKETPLACE_PORT, debug=False)
