import logging
import os
import sys

from api.v1 import blueprints
from db.mongodb_client import mongo_init
from flask import Flask
from flask_cors import CORS
from flask_smorest import Api
from flask_swagger_ui import get_swaggerui_blueprint

# Configure logging with environment variable, default to DEBUG
log_level_str = os.environ.get("LOG_LEVEL", "DEBUG").upper()
log_level = getattr(logging, log_level_str, logging.DEBUG)

logging.basicConfig(
    level=log_level,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
    handlers=[
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger("resource_abstractor")
logger.setLevel(log_level)

# Suppress mongo debug logs
logging.getLogger("pymongo").setLevel(logging.WARNING)
logging.getLogger("pymongo.connection").setLevel(logging.WARNING)
logging.getLogger("pymongo.serverSelection").setLevel(logging.WARNING)

RESOURCE_ABSTRACTOR_PORT = os.environ.get("RESOURCE_ABSTRACTOR_PORT")

app = Flask(__name__)
app.logger.setLevel(log_level)

# Configure CORS with explicit settings
CORS(app, resources={
    r"/api/*": {
        "origins": "*",
        "methods": ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
        "allow_headers": ["Content-Type", "Authorization"],
        "expose_headers": ["Content-Type"],
        "supports_credentials": False
    }
})

# Disable strict slashes to prevent redirects
app.url_map.strict_slashes = False

app.config["OPENAPI_VERSION"] = "3.0.2"
app.config["API_TITLE"] = "Resource Abstractor Api"
app.config["API_VERSION"] = "v1"
app.config["OPENAPI_URL_PREFIX"] = "/docs"

api = Api(app)
mongo_init(app)

# Register blueprints
SWAGGER_URL = "/api/docs"
API_URL = "/docs/openapi.json"
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={"app_name": "Resource Abstractor"},
)
app.register_blueprint(swaggerui_blueprint)

for blp in blueprints:
    api.register_blueprint(blp)


@app.route("/", methods=["GET"])
def health():
    return "ok"


if __name__ == "__main__":
    app.run(host="::", port=RESOURCE_ABSTRACTOR_PORT, debug=False)
