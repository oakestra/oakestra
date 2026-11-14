import os
from datetime import datetime, timezone

from api.v1.addons_api import addonsblp
from api.v1.ui_addons_api import uiaddonsblp
from bson.objectid import ObjectId
from db.mongodb_client import mongo_init
from flask import Flask
from flask.json import JSONEncoder
from flask_cors import CORS
from flask_smorest import Api
from flask_swagger_ui import get_swaggerui_blueprint

ADDONS_MANAGER_PORT = int(os.environ.get("ADDONS_MANAGER_PORT", 11101))


class AddonsJSONEncoder(JSONEncoder):
    def default(self, o):
        if isinstance(o, ObjectId):
            return str(o)
        if isinstance(o, datetime):
            # pymongo returns naive datetimes because the client isn't tz_aware. We only store
            # UTC, so add the offset, otherwise browsers parse the value as local time.
            if o.tzinfo is None:
                o = o.replace(tzinfo=timezone.utc)
            return o.isoformat()
        return super().default(o)


app = Flask(__name__)
app.json_encoder = AddonsJSONEncoder

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
app.config["API_TITLE"] = "Addon Manager Api"
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
    config={"app_name": "Addon Manager"},
)
app.register_blueprint(swaggerui_blueprint)
api.register_blueprint(addonsblp)
api.register_blueprint(uiaddonsblp)


@app.route("/", methods=["GET"])
def health():
    return "ok"


if __name__ == "__main__":
    app.run(host="::", port=ADDONS_MANAGER_PORT, debug=False)
