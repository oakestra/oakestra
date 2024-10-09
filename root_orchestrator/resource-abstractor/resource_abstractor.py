import os

from api.v1 import blueprints
from db.mongodb_client import mongo_init
from flask import Flask
from flask_smorest import Api
from flask_swagger_ui import get_swaggerui_blueprint

RESOURCE_ABSTRACTOR_PORT = os.environ.get("RESOURCE_ABSTRACTOR_PORT")

app = Flask(__name__)

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
