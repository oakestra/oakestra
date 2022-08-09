import json

from bson import json_util
from flask import flash, request
from flask_cors import CORS
from flask_jwt_extended import JWTManager, decode_token
from jwt import InvalidTokenError, DecodeError

from blueprints import blueprints
from flask_socketio import SocketIO, emit
from pathlib import Path
from werkzeug.utils import secure_filename, redirect

from ext_requests.cluster_db import mongo_find_by_name_and_location, mongo_update_pairing_complete
from ext_requests.mongodb_client import mongo_init
from ext_requests.net_plugin_requests import *
from ext_requests.user_db import create_admin
from roles.securityUtils import check_jwt_token_validity
from sm_logging import configure_logging
from flask import Flask
from secrets import token_hex
from datetime import timedelta
from flask_smorest import Api
from flask_swagger_ui import get_swaggerui_blueprint

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

    '''create a new method (i.e.: mongo_pair_cluster with the content of the message, update the parameters)'''

    net_port = message['network_component_port']
    del message['manager_port']
    del message['network_component_port']
    app.logger.info("MONGODB - checking if the cluster introduced is in our Database...")
    existing_cl = mongo_find_by_name_and_location(message)
    if existing_cl is None:
        response = {
            'error': "The cluster you are trying to pair is not yet saved in our database, please log in in the "
                     "Dashboard and add your cluster there. "
        }
    elif existing_cl['pairing_complete']:
        app.logger.info("The pairing was already completed")
        # TODO: Authenticate with shared key that cluster must have after the first running
        # TODO: if key expired --> Ask for user credentials
        response = {
            'error': "Your cluster has already been attached to the Root Orchestrator. Id of cluster: " + str(
                existing_cl['_id'])
        }
    else:
        '''try:
            token_info = decode_token(encoded_token=message['pairing_key'])
        except (InvalidTokenError, DecodeError):
            raise Exception('Token not found')
        if token_info is not None:'''
        # take into consideration "NOT ENOUGH SEGMENTS" DECODE CATCH ERROR

        token_info = check_jwt_token_validity(message['pairing_key'])
        if token_info is None:
            response = {
                'error': "Pairing key not found"
            }
        elif existing_cl['pairing_key'] == message['pairing_key']:
            app.logger.info("The keys match")
            # TODO: Consider the case where the key is expired
            response = {
                'id': str(existing_cl['_id'])
            }
            mongo_update_pairing_complete(existing_cl['_id'])
            net_register_cluster(
                cluster_id=str(existing_cl['_id']),
                cluster_address=request.remote_addr,
                cluster_port=net_port
            )
            # TODO: Creates the shared secret key (with expiration date too) and send it to the cluster

            #dt_string = datetime.now().strftime("%d/%m/%Y %H:%M:%S")
            #data = {"iat": dt_string, "aud": "SecretClusterKey", "cluster_name": existing_cl['cluster_name'],
            #        "num": str(randint(0, 99999999))}
            #create_access_token(identity=existing_cl['_id'], expires_delta=timedelta(days=30), additional_claims=data)
        else:
            app.logger.info("The pairing does not match")
            response = {
                'error': "Your pairing key does not match the one generated for your cluster"
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
