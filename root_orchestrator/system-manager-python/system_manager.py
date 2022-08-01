import json
import threading

from bson import json_util, ObjectId
from flask import flash, request
from flask_cors import CORS
from flask_jwt_extended import JWTManager

from blueprints import blueprints
from flask_socketio import SocketIO, emit
from markupsafe import escape
from pathlib import Path
from werkzeug.utils import secure_filename, redirect

# from blueprints.applications_blueprints import *
# from blueprints.authentication_blueprints import *
# from blueprints.authorization_blueprints import *
# from blueprints.deployment_blueprints import DeployInstanceController
from ext_requests.apps_db import mongo_update_job_status_and_instances, mongo_find_job_by_id
from ext_requests.cluster_db import mongo_update_cluster_information, mongo_get_all_clusters, \
    mongo_find_all_active_clusters, mongo_find_cluster_by_location, mongo_find_cluster_by_id_and_set_number_of_nodes, \
    mongo_upsert_cluster, mongo_verify_pairing_key
from ext_requests.cluster_requests import *
from ext_requests.mongodb_client import mongo_init
from ext_requests.net_plugin_requests import *
from ext_requests.scheduler_requests import scheduler_request_deploy, scheduler_request_replicate, \
    scheduler_request_status
from ext_requests.user_db import create_admin
from sm_logging import configure_logging
from flask import Flask
from secrets import token_hex
from datetime import timedelta
from flask_smorest import Blueprint, Api, abort
from flask_swagger_ui import get_swaggerui_blueprint

# from blueprints.users_blueprints import *

my_logger = configure_logging()

UPLOAD_FOLDER = 'files'
ALLOWED_EXTENSIONS = {'txt', 'json', 'yml'}

app = Flask(__name__)

app.config['OPENAPI_VERSION'] = '3.0.2'
app.config['API_TITLE'] = 'Oakestra root api'
app.config['API_VERSION'] = 'v1'
app.config["OPENAPI_URL_PREFIX"] = '/docs'
app.config['UPLOAD_FOLDER'] = UPLOAD_FOLDER
app.config["JWT_SECRET_KEY"] = token_hex(32)
app.config["JWT_ACCESS_TOKEN_EXPIRES"] = timedelta(minutes=10)
app.config["JWT_REFRESH_TOKEN_EXPIRES"] = timedelta(days=7)
app.config["RESET_TOKEN_EXPIRES"] = timedelta(hours=3)  # for password reset

jwt = JWTManager(app)
api = Api(app, spec_kwargs={"host": "oakestra.io", "x-internal-id": "1"})

cors = CORS(app, resources={r"/*": {"origins": "*"}})
socketio = SocketIO(app, async_mode='eventlet', logger=True, engineio_logger=True, cors_allowed_origins='*')
mongo_init(app)
create_admin()

MY_PORT = os.environ.get('MY_PORT') or 10000

cluster_gauges_for_prometheus = []

# Register apis
for bp in blueprints:
    api.register_blueprint(bp)

api.spec.components.security_scheme(
    "bearerAuth", {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}
)
api.spec.options["security"] = [{"bearerAuth": []}]

# Swagger docs
SWAGGER_URL = '/api/docs'
API_URL = '/docs/openapi.json'
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={
        'app_name': "Oakestra root orchestrator"
    },
)
app.register_blueprint(swaggerui_blueprint)


# .......... Register clusters via WebSocket ...........#
# ......................................................#

@socketio.on('connect', namespace='/register')
def on_connect():
    app.logger.info('SocketIO - Cluster_Manager connected: {}'.format(request.remote_addr))
    app.logger.info(request.environ.get('REMOTE_PORT'))
    # time.sleep(1)  #  Wait here to Avoid Race Condition with Client (Cluster Manager) does no work.
    # Apparently, nothing in between is sent by Websocket protocol
    emit('sc1', {'Hello-Cluster_Manager': 'please send your cluster info'}, namespace='/register')


@socketio.on('cs1', namespace='/register')
def handle_init_client(message):
    app.logger.info('SocketIO - Received Cluster_Manager_to_System_Manager_1: {}:{}'.
                    format(request.remote_addr, request.environ.get('REMOTE_PORT')))
    app.logger.info(message)

    '''create a new method (i.e.: mongo_pair_cluster with the content of the message, update de parameters)'''

    cid = mongo_upsert_cluster(cluster_ip=request.remote_addr, message=message)

    if mongo_verify_pairing_key(message['userId'] + cid, message['pairing_key']):
        response = {
            'id': str(cid)
        }
        net_register_cluster(
            cluster_id=str(cid),
            cluster_address=request.remote_addr,
            cluster_port=message['network_component_port']
        )
        # TODO: We have to invalidate the key
    else:
        response = {
            'error': "the pairing key introduced does not match"
        }

    emit('sc2', json.dumps(response), namespace='/register')


@socketio.event(namespace='/register')
def disconnect():
    app.logger.info('SocketIO - Client disconnected')


# ............... Finish WebSocket handling ............#
# ......................................................#

def allowed_file(filename):
    return '.' in filename and \
           filename.rsplit('.', 1)[1].lower() in ALLOWED_EXTENSIONS


# Used to upload file from the frontend
@app.route('/frontend/uploader', methods=['GET', 'POST'])
def upload_file():
    if request.method == 'POST':
        if 'file' not in request.files:
            flash('No file part')
            return redirect(request.url)
        file = request.files['file']
        if file.filename == '':
            flash('No selected file')
            return redirect(request.url)
        if not allowed_file(file.filename):
            return "Not a valid file", 400
        if file:
            filename = secure_filename(file.filename)
            file.save(os.path.join(app.config['UPLOAD_FOLDER'], filename))
            response = {"path": str(Path(filename).absolute())}
            return str(json_util.dumps(response))
    return '''
    <!doctype html>
    <h1>Not a valid request</h1>
    '''


if __name__ == '__main__':
    import eventlet

    eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)
