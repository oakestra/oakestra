import json
import traceback
from random import randint

from bson import json_util
from flask import flash, request
from flask_cors import CORS
from flask_jwt_extended import JWTManager
from jwt import ExpiredSignatureError

from blueprints import blueprints
from flask_socketio import SocketIO, emit
from pathlib import Path
from werkzeug.utils import secure_filename, redirect

from ext_requests.cluster_db import mongo_find_by_name_and_location, mongo_update_pairing_complete
from ext_requests.mongodb_client import mongo_init
from ext_requests.net_plugin_requests import *
from ext_requests.user_db import create_admin
from roles.securityUtils import check_jwt_token_validity, create_jwt_secret_key_cluster, jwt_auth_required, \
    create_jwt_refresh_secret_key_cluster
from sm_logging import configure_logging
from flask import Flask
from secrets import token_hex
from datetime import timedelta, datetime, timezone
from flask_smorest import Api
from flask_swagger_ui import get_swaggerui_blueprint

from users.auth import user_token_refresh

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
app.config["JWT_CLUSTER_ACCESS_TOKEN_EXPIRES"] = timedelta(hours=5)  # not used as it is inaccessible from securityUtils

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


def fill_additional_claims(cluster, aud):
    return {"iat": datetime.now(), "aud": aud,
            "sub": str(cluster['_id']), # TODO: change to username
            "clusterName": cluster['cluster_name'],
            "latitude": cluster['cluster_latitude'], # TODO: delete locationa param
            "longitude": cluster['cluster_longitude'],
            "num": str(randint(0, 99999999))}


# TODO: Test this function
def is_key_expiring(exp):
    now = datetime.now(timezone.utc)
    target_timestamp = datetime.timestamp(now + timedelta(days=1))
    if target_timestamp > exp:
        return True
    else:
        return False


def check_if_keys_match(token_info, message):
    if token_info.get("clusterName") == message['cluster_name'] and token_info.get("latitude") == message[
        'cluster_latitude'] and token_info.get("longitude") == message['cluster_longitude']:
        return True
    else:
        return False


def token_validation(message, key_type, cluster, net_port):
    cluster_id = str(cluster['_id'])
    try:
        token_info = check_jwt_token_validity(message[key_type])

        if check_if_keys_match(token_info, message):
            app.logger.info("The keys match")
            response = {
                'id': cluster_id
            }
            # Make pairing complete, register cluster and generate shared secret key with cluster to secure each
            # further connection
            if key_type == 'pairing_key':
                mongo_update_pairing_complete(cluster_id)

                net_register_cluster(
                    cluster_id=cluster_id,
                    cluster_address=request.remote_addr,
                    cluster_port=net_port
                )

                claims = fill_additional_claims(cluster, "connectCluster")
                secret_key = create_jwt_secret_key_cluster(cluster_id, timedelta(days=5), claims)
                refresh_secret_key = create_jwt_refresh_secret_key_cluster(cluster_id, timedelta(days=30))
            else:
                if is_key_expiring(token_info["exp"]):
                    # TODO: first check refresh and then access token (secret key)
                    claims = fill_additional_claims(cluster, "connectCluster")
                    secret_key = create_jwt_secret_key_cluster(cluster_id, timedelta(days=5), claims)
                else:
                    secret_key = message[key_type]
            response['secret_key'] = secret_key

        else:
            app.logger.info("The pairing does not match")
            response = {
                'error': "Your pairing key does not match the one generated for your cluster"
            }
    except Exception as e:
        print(traceback.format_exc())
        if str(e) == "No token supplied" or str(e) == "Not enough segments" or str(e) == "Too many segments":
            response = {
                'error': "The key introduced is invalid"
            }
        elif str(e) == "Signature verification failed" or str(e) == "Algorithm not supported":
            response = {
                'error': "integrity-error"
            }
        elif e == ExpiredSignatureError:
            if key_type == 'pairing_key':
                response = {
                    'error': "Your cluster's pairing key has expired; please log in again to the Dashboard to ask "
                             "again to attach your cluster. "
                }
            else:  # key_type == 'secret_key'
                try:
                    # TODO: Before creating a new secret key, check the validation of the refresh token (created at
                    #  the same time as the secret key == access token)
                    # A new token is generated in case of the old one expired
                    claims = fill_additional_claims(cluster, "connectCluster")
                    new_secret_key = create_jwt_secret_key_cluster(cluster_id, timedelta(days=5), claims)
                    response = {
                        'warning': "Your cluster's secret key had expired; you now have a new one.",
                        'secret_key': new_secret_key
                    }
                    # TODO: Ask to log in the terminal with socketio
                except Exception as e:
                    if e == ExpiredSignatureError:
                        response = {
                            'error': "Your cluster's secret key has expired; please log in again to get a new one. "
                        }
                    else:
                        response = {
                            'error': str(e)
                        }
        else:
            response = {
                'error': str(e)
            }
    return response


@socketio.on('connect', namespace='/register')
def on_connect():
    app.logger.info('SocketIO - Cluster_Manager connected: {}'.format(request.remote_addr))
    app.logger.info(request.environ.get('REMOTE_PORT'))
    # time.sleep(1)  #  Wait here to Avoid Race Condition with Client (Cluster Manager) does no work.
    # Apparently, nothing in between is sent by Websocket protocol
    emit('sc1', {'Hello-Cluster_Manager': 'please send your cluster info'}, namespace='/register')


@jwt_auth_required()
@socketio.on('cs1', namespace='/register')
def handle_init_client(message):
    app.logger.info('SocketIO - Received Cluster_Manager_to_System_Manager_1: {}:{}'.
                    format(request.remote_addr, request.environ.get('REMOTE_PORT')))
    app.logger.info(message)
    net_port = message['network_component_port']
    del message['manager_port']
    del message['network_component_port']
    app.logger.info("MONGODB - checking if the cluster introduced is in our Database...")
    existing_cl = mongo_find_by_name_and_location(message)
    if existing_cl is None:
        response = {
            'error': "The cluster is not yet saved, please log in in the Dashboard and add your cluster there. "
        }
    elif existing_cl['pairing_complete']:
        if message['secret_key'] is "":
            response = {
                'warning': "The pairing was already completed; please authenticate with the cluster's secret key",
                'id': str(existing_cl['_id'])
            }
        else:
            response = token_validation(message, 'secret_key', existing_cl, net_port)
    else:
        response = token_validation(message, 'pairing_key', existing_cl, net_port)

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
