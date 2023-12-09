import os
from flask import Flask
from flask_smorest import Api
from flask_swagger_ui import get_swaggerui_blueprint

from api.v1 import blueprints
from db.mongodb_client import mongo_init

MY_PORT = os.environ.get("MY_PORT") or 10009

app = Flask(__name__)

app.config['OPENAPI_VERSION'] = '3.0.2'
app.config['API_TITLE'] = 'Resource Abstractor Api'
app.config['API_VERSION'] = 'v1'
app.config["OPENAPI_URL_PREFIX"] = '/docs'

api = Api(app)
mongo_init(app)

# Register blueprints
SWAGGER_URL = '/api/docs'
API_URL = '/docs/openapi.json'
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={
        'app_name': "Resource Abstractor"
    },
)
app.register_blueprint(swaggerui_blueprint)

for blp in blueprints:
    api.register_blueprint(blp)

@app.route('/', methods=['GET'])
def hello_world():
    return "Hello, World! This is the Resource Abstractor.\n"

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=MY_PORT, debug=False)
