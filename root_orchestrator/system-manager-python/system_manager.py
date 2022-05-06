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
    mongo_upsert_cluster
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


# ........ Endpoints for the authentication.............#
# ......................................................#

# api.add_resource(UserResetPasswordController, '/api/auth/resetPassword')
#
# # .......... Endpoints for the user functions ..........#
# # ......................................................#
# api.add_resource(UserController, '/api/user/<string:username>')
# api.add_resource(AllUserController, '/api/users')
# api.add_resource(UserChangePasswordController, '/api/changePassword/<string:username>')
# api.add_resource(UserRolesController, '/api/roles')


@app.route('/api/result/deploy', methods=['POST'])
def receive_scheduler_result_and_propagate_to_cluster():
    app.logger.info('Incoming Request /api/result/deploy - received cloud_scheduler result')
    data = json.loads(request.json)
    # Omit worker nodes coordinates to avoid flooding the log
    data_without_worker_groups = data.copy()
    data_without_worker_groups['cluster'].pop('worker_groups', None)
    app.logger.info(data_without_worker_groups)
    system_job_id = data.get('job_id')
    replicas = data.get('replicas')
    cluster_id = str(data.get('cluster').get('_id').get('$oid'))

    # Updating status and instances
    instance_list = []
    for i in range(replicas):
        instance_info = {
            'instance_number': i,
            'cluster_id': cluster_id,
        }
        instance_list.append(instance_info)

    # Inform network plugin about the deployment
    threading.Thread(group=None, target=net_inform_instance_deploy,
                     args=(str(system_job_id), replicas, cluster_id)).start()

    # Update the current instance information
    mongo_update_job_status_and_instances(
        job_id=system_job_id,
        status='CLUSTER_SCHEDULED',
        replicas=replicas,
        instance_list=instance_list
    )

    cluster_request_to_deploy(data.get('cluster'), mongo_find_job_by_id(system_job_id))
    return "ok"


@app.route('/api/information/<cluster_id>', methods=['GET', 'POST'])
def cluster_information(cluster_id):
    if request.method == 'POST':
        """Endpoint to receive aggregated information of a Cluster Manager"""
        app.logger.info(
            'Incoming Request /api/information/{0} to set aggregated cluster information'.format(cluster_id))
        # data = json.loads(request.json)
        data = request.json  # data contains cpu_percent, memory_percent, cpu_cores etc.
        # Omit worker nodes coordinates to avoid flooding the log
        data_without_worker_groups = data.copy()
        data_without_worker_groups.pop('worker_groups', None)
        app.logger.info(data_without_worker_groups)
        mongo_update_cluster_information(cluster_id, data)
        return "ok", 200
    return "no file", 200


# ................ Scheduler Test .....................#
# .....................................................#


@app.route('/api/test/scheduler', methods=['GET'])
def scheduler_test():
    app.logger.info('Incoming Request /api/jobs - to get all jobs')
    return scheduler_request_status()


# ................ Clusters Endpoints ................#
# ....................................................#


@app.route('/api/clusters_all', methods=['GET'])
def get_all_clusters():
    app.logger.info('Incoming Request /api/clusters_all - to get all known clusters')
    return str(json_util.dumps(mongo_get_all_clusters()))


@app.route('/api/clusters', methods=['GET'])
def get_active_clusters():
    app.logger.info('Incoming Request /api/clusters - to get all active clusters')
    return str(json_util.dumps(mongo_find_all_active_clusters()))


@app.route('/api/clusters/count', methods=['GET'])
def get_number_of_clusters():
    return "ok"


@app.route('/api/cluster/<c_id>/nodes/<number_of_nodes>')
def set_node(c_id, number_of_nodes):
    app.logger.info('Incoming Request /api/cluster/{0}/nodes/{1} - to set number of nodes in a cluster'.
                    format(escape(c_id), escape(number_of_nodes)))

    app.logger.info(escape(c_id))
    mongo_find_cluster_by_id_and_set_number_of_nodes(ObjectId(c_id), number_of_nodes)
    return "ok"


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

    cid = mongo_upsert_cluster(cluster_ip=request.remote_addr, message=message)
    x = {
        'id': str(cid)
    }

    net_register_cluster(
        cluster_id=str(cid),
        cluster_address=request.remote_addr,
        cluster_port=message['network_component_port']
    )

    emit('sc2', json.dumps(x), namespace='/register')


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
