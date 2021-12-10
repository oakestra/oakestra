import os
from flask import Flask, flash, request, jsonify
from flask_socketio import SocketIO, emit
import json
from bson.objectid import ObjectId
from markupsafe import escape
import time
import threading
from net_plugin_requests import *
from bson import json_util
from mongodb_client import *
from yamlfile_parser import yaml_reader
from cluster_requests import *
from scheduler_requests import scheduler_request_deploy, scheduler_request_replicate, scheduler_request_status
from net_plugin_requests import *
from sm_logging import configure_logging

my_logger = configure_logging()

app = Flask(__name__)
app.secret_key = b'\xc8I\xae\x85\x90E\x9aBxQP\xde\x8es\xfdY'

socketio = SocketIO(app, async_mode='eventlet', logger=True, engineio_logger=True, cors_allowed_origins='*')

mongo_init(app)

MY_PORT = os.environ.get('MY_PORT') or 10000

cluster_gauges_for_prometheus = []


# ............ Flask API Endpoints .....................#
# ......................................................#


@app.route('/')
def hello_world():
    app.logger.info("Hello World Request.")
    return "Hello, World! This is Systems Manager's REST API"


@app.route('/sample')
def sample():
    app.logger.info("sample Request.")
    time.sleep(2)
    return "sample", 200


@app.route('/status')
def status():
    app.logger.info("Incoming Request /status")
    return "ok", 200


# ......... Deployment Endpoints .......................#
# ......................................................#


@app.route('/api/result/deploy', methods=['POST'])
def receive_scheduler_result_and_propagate_to_cluster():
    app.logger.info('Incoming Request /api/result/deploy - received cloud_scheduler result')
    data = json.loads(request.json)
    app.logger.info(data)
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
    threading.Thread(group=None, target=net_inform_instance_deploy, args=(str(system_job_id),replicas,cluster_id)).start()

    #Update the current instance information
    mongo_update_job_status_and_instances(
        job_id=system_job_id,
        status='CLUSTER_SCHEDULED',
        replicas=replicas,
        instance_list=instance_list
    )

    cluster_request_to_deploy(data.get('cluster'), mongo_find_job_by_id(system_job_id))
    return "ok"


@app.route('/api/result/replicate', methods=['POST'])
def receive_scheduler_replicate_result_and_propagate_to_cluster():
    """
    Replication function not yet fully implemented
    """
    app.logger.info('Incoming Request /api/result/replicate - received cloud_scheduler result')
    data = json.loads(request.json)
    system_job_id = data.get('job_id')
    job = data.get('job')
    replicas = data.get('replicas')

    return 'ok', 200


@app.route('/api/feedback')
def get_cluster_feedback():
    """whether job is running or not"""
    app.logger.info("Incoming Request /api/feedback")
    return "ok"


@app.route('/api/information/<cluster_id>', methods=['GET', 'POST'])
def cluster_information(cluster_id):
    """Endpoint to receive aggregated information of a Cluster Manager"""
    app.logger.info('Incoming Request /api/information/{0} to set aggregated cluster information'.format(cluster_id))
    # data = json.loads(request.json)
    data = request.json  # data contains cpu_percent, memory_percent, cpu_cores etc.
    app.logger.info(data)
    mongo_update_cluster_information(cluster_id, data)
    return "ok", 200


@app.route('/api/deploy', methods=['POST'])
def deploy_task():
    app.logger.info('Incoming Request /api/deploy - deploying task...')

    if 'file' not in request.files:
        flash('No file part')
        return "no file", 400
    file = request.files['file']
    app.logger.info('file found')
    if file.filename == '':
        flash('No selected file')
        return "empty file", 400
    if file:
        # Reading config file
        data = yaml_reader(file)
        app.logger.info(data)
        # Insert job into database
        job_id = mongo_insert_job(
            {
                'file_content': data
            })
        # Inform network plugin about the deployment
        threading.Thread(group=None, target=net_inform_service_deploy, args=(data, str(job_id),)).start()
        # Job status to scheduling REQUESTED
        mongo_update_job_status(job_id, 'REQUESTED')
        # Request scheduling
        threading.Thread(group=None, target=scheduler_request_deploy, args=(data, str(job_id),)).start()
        return {'job_id': str(job_id)}, 200



# ................ Scheduler Test .....................#
# .....................................................#


@app.route('/api/test/scheduler', methods=['GET'])
def scheduler_test():
    app.logger.info('Incoming Request /api/jobs - to get all jobs')
    return scheduler_request_status()


# ................ Jobs Endpoints .....................#
# .....................................................#

@app.route('/api/jobs', methods=['GET'])
def get_jobs():
    app.logger.info('Incoming Request /api/jobs - to get all jobs')
    return str(json_util.dumps(mongo_get_all_jobs()))


@app.route('/api/job/status/<job_id>')
def job_status(job_id):
    app.logger.info('Incoming Request /api/job/status/{0} - to get job status'.format(escape(job_id)))
    return mongo_get_job_status(escape(job_id))


@app.route('/api/delete/<job_id>')
def delete_task(job_id):
    app.logger.info('Incoming Request /api/delete/{0} to delete task...'.format(escape(job_id)))
    # find service in db and ask corresponding cluster to delete task
    cluster_obj = mongo_find_cluster_of_job(job_id)
    cluster_request_to_delete_job(cluster_obj, job_id)
    return "ok\n", 200


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


@app.route('/api/clusters/move', methods=['GET', 'POST'])
def move_from_cluster_to_cluster():
    """move a service from one cluster to another"""
    """request body should be in this format: {job_id: j, from_location: f, to_location: t}"""
    app.logger.info('Incoming Request /api/move - to move service from one cluster to another')

    data = request.json
    job_id = data.get('job_id')
    from_cluster = data.get('from_location')
    to_cluster = data.get('to_location')
    job_obj = mongo_find_job_by_id(job_id)
    from_cluster_obj = mongo_find_cluster_by_location(from_cluster)
    to_cluster_obj = mongo_find_cluster_by_location(to_cluster)

    if from_cluster_obj == 'Error':
        return 'Error: Source Cluster does not exist', 404
    if to_cluster_obj == 'Error':
        return 'Error: Target Cluster does not exist', 404

    cluster_request_to_delete_job(cluster_obj=from_cluster_obj, job_id=job_id)
    cluster_request_to_deploy(cluster_obj=to_cluster_obj, job=job_obj)
    return "ok\n", 200


@app.route('/api/nodes/move', methods=['GET', 'POST'])
def move_within_cluster():
    """Move a service from one node to another within the same cluster"""
    data = request.json
    job_id = data.get('job_id')
    cluster = data.get('cluster')
    node_source = data.get('from_node')
    node_target = data.get('to_node')
    job_obj = mongo_find_job_by_id(job_id)
    cluster_obj = mongo_find_cluster_by_location(cluster)
    if cluster_obj is None:
        return "ClusterNotFound\n", 404
    cluster_request_to_move_within_cluster(cluster_obj, job_id, node_source, node_target)
    return "Ok\n", 200


@app.route('/api/replicate_up', methods=['GET', 'POST'])
def replicate_up():
    """replicate service"""
    app.logger.info("Incoming Request /api/replicate")
    data = request.json
    job_id = data.get('job_id')
    replicas_desired = int(data.get('replicas'))
    job_obj = mongo_find_job_by_id(job_id)

    replicas_current = job_obj.get('replicas')
    cluster_obj_of_job = mongo_find_cluster_of_job(job_id)

    cluster_request_to_replicate_up(cluster_obj_of_job, job_obj, replicas_desired)

    return "ok"


@app.route('/api/replicate_down/<job_id>/<replicas>', methods=['GET', 'POST'])
def replicate_down(job_id, replicas):
    """shrink numbers of service"""
    # find cluster of service and ask that cluster to shrink number of deployments to desired amount
    cluster_obj = mongo_find_cluster_of_job(job_id=job_id)
    cluster_request_to_replicate_down(cluster_obj=cluster_obj, job_obj=cluster_obj, int_replicas=replicas)
    return "ok"


@app.route('/api/replicate', methods=['GET', 'POST'])
def replicate():
    app.logger.info('Incoming Request /api/replicate')
    data = request.json
    job_id = data.get('job_id')
    replicas_desired = int(data.get('replicas'))
    job_obj = mongo_find_job_by_id(job_id)
    scheduler_request_replicate(job_obj, replicas_desired)
    return 'ok', 200


@app.route('/api/cluster/<c_id>/incr_node')
def increase_node(c_id):
    app.logger.info('increment node of cluster')
    app.logger.info(escape(c_id))
    mongo_find_cluster_by_id_and_incr_node(ObjectId(c_id))
    return "ok"


@app.route('/api/cluster/<c_id>/nodes/<number_of_nodes>')
def set_node(c_id, number_of_nodes):
    app.logger.info('Incoming Request /api/cluster/{0}/nodes/{1} - to set number of nodes in a cluster'.
                    format(escape(c_id), escape(number_of_nodes)))

    app.logger.info(escape(c_id))
    mongo_find_cluster_by_id_and_set_number_of_nodes(ObjectId(c_id), number_of_nodes)
    return "ok"


@app.route('/api/cluster/<c_id>/decr_node')
def decrease_node(c_id):
    app.logger.info('Incoming Request /api/cluster/{0}/decr_node - to increment node of cluster'.format(escape(c_id)))
    app.logger.info(escape(c_id))
    mongo_find_cluster_by_id_and_decr_node(ObjectId(c_id))
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

    emit('sc2', json.dumps(x), namespace='/register')


@socketio.event(namespace='/register')
def disconnect():
    app.logger.info('SocketIO - Client disconnected')


# ............... Finish WebSocket handling ............#
# ......................................................#


if __name__ == '__main__':
    print('moin')
    # start_http_server(10008)

    # socketio.run(app, debug=True, host='0.0.0.0', port=MY_PORT)
    import eventlet

    eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)
