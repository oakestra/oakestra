from flask_cors import CORS
from blueprints import blueprints
from flask_smorest import Api
from flask import Flask
from flask_swagger_ui import get_swaggerui_blueprint
import os


app = Flask(__name__)

app.config['OPENAPI_VERSION'] = '3.0.2'
app.config['API_TITLE'] = 'coordinate system'
app.config['API_VERSION'] = 'v1'
app.config["OPENAPI_URL_PREFIX"] = '/docs'


api = Api(app, spec_kwargs={"host": "oakestra.io", "x-internal-id": "1"})
cors = CORS(app, resources={r"/*": {"origins": "*"}})

MY_PORT = os.environ.get('MY_PORT') or 10106


# Register apis
for bp in blueprints:
    api.register_blueprint(bp)

# Swagger docs
SWAGGER_URL = '/api/docs'
API_URL = '/docs/openapi.json'
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={
        'app_name': "coordinate system"
    },
)
app.register_blueprint(swaggerui_blueprint)