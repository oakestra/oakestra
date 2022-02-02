from flask import Flask, flash, request
from flask_socketio import SocketIO, emit
import json
from markupsafe import escape
import threading
from bson import json_util

from mongodb_client import *
from sla_parser import parse_sla

from service_manager import new_instance_ip, clear_instance_ip, service_resolution, new_subnetwork_addr, \
    service_resolution_ip, new_job_rr_address
from cluster_requests import *
from scheduler_requests import scheduler_request_deploy, scheduler_request_replicate, scheduler_request_status
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
    # Omit worker nodes coordinates to avoid flooding the log
    data_without_worker_groups = data.copy()
    data_without_worker_groups['cluster'].pop('worker_groups', None)
    app.logger.info(data_without_worker_groups)
    system_job_id = data.get('job_id')
    replicas = data.get('replicas')
    resulting_cluster = data.get('cluster')
    # mongo_update_job_status(job_id, 'SCHEDULED')  # done in cloud-scheduler already

    # Updating status and instances
    instance_list = []
    for i in range(replicas):
        instance_info = {
            'instance_number': i,
            'instance_ip': new_instance_ip(),
            'cluster_id': str(resulting_cluster.get('_id').get('$oid')),
            'namespace_ip': '',
            'host_ip': '',
            'host_port': '',
        }
        instance_list.append(instance_info)
    mongo_update_job_status_and_instances(
        job_id=system_job_id,
        status='CLUSTER_SCHEDULED',
        replicas=replicas,
        instance_list=instance_list
    )

    cluster_request_to_deploy(resulting_cluster, mongo_find_job_by_id(system_job_id))
    return "ok"


@app.route('/api/result/cluster_deploy', methods=['POST'])
def get_cluster_deployment_status_feedback():
    """
    Result of the deploy operation in a cluster
    json file structure:{
        'job_id':string
        'instances:[{
            'instance_number':int
            'namespace_ip':string
            'host_ip':string
            'host_port':string
        }]
    }
    """
    app.logger.info("Incoming Request /api/result/cluster_deploy")
    data = request.json
    app.logger.info(data)

    mongo_update_job_net_status(
        job_id=data.get('job_id'),
        instances=data.get('instances')
    )

    return "roger that"


@app.route('/api/result/replicate', methods=['POST'])
def receive_scheduler_replicate_result_and_propagate_to_cluster():
    app.logger.info('Incoming Request /api/result/deploy - received cloud_scheduler result')
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
    # Omit worker nodes coordinates to avoid flooding the log
    data_without_worker_groups = data.copy()
    data_without_worker_groups.pop('worker_groups', None)
    app.logger.info(data_without_worker_groups)
    mongo_update_cluster_information(cluster_id, data)
    return "ok", 200


@app.route('/api/deploy', methods=['GET', 'POST'])
def deploy_task():
    app.logger.info('Incoming Request /api/deploy - deploying task...')

    if request.method == 'POST':
        app.logger.info('POST request')

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
            # data = yaml_reader(file)
            # print(f"XXX: {json.loads(file.read())}")
            data = parse_sla(file)
            app.logger.info(data)
            job_ids = {}

            applications = data.get('applications')
            app.logger.info(f"SLA DATA: {data}")
            for application in applications:
                application_name = application.get('application_name')
                microservices = application.get('microservices')
                for i, microservice in enumerate(microservices):
                    app.logger.info(f"Process microservice {i+1}/{len(microservices)}")
                    # Assigning a Service IP for RR Load Balancing
                    s_ip = [{
                        "IpType": 'RR',
                        "Address": new_job_rr_address(application, microservice),
                    }]
                    # Insert job into database
                    job_id = mongo_insert_job(
                        {
                            'file_content': {'application': application, 'microservice': microservice},
                            'service_ip_list': s_ip
                        })
                    app.logger.info(f"Inserted Job with ID: {str(job_id)}")
                    # Remove other microservices from deployment job
                    application.pop("microservices")
                    job = {**application, **microservice}
                    # Request scheduling
                    threading.Thread(group=None, target=scheduler_request_deploy,
                                     args=(job, str(job_id))).start()
                    # Job status to scheduling REQUESTED
                    mongo_update_job_status(job_id, 'REQUESTED')

                    job_ids.setdefault(application_name, []).append(job_id)

            return job_ids, 200

    return ("/api/deploy request wihout a yaml file\n", 200)


# ............. Network management Endpoint ............#
# ......................................................#

@app.route('/api/job/<job_name>/instances', methods=['GET'])
def table_query_resolution_by_jobname(job_name):
    """
    Get all the instances of a job given the complete name
    """
    job_name = job_name.replace("_", ".")
    app.logger.info("Incoming Request /api/job/" + str(job_name) + "/instances")
    return {'instance_list': service_resolution(job_name)}


@app.route('/api/job/ip/<service_ip>/instances', methods=['GET'])
def table_query_resolution_by_ip(service_ip):
    """
    Get all the instances of a job given a Service IP in 172_30_x_y notation
    """
    service_ip = service_ip.replace("_", ".")
    app.logger.info("Incoming Request /api/job/ip/" + str(service_ip) + "/instances")
    return {'instance_list': service_resolution_ip(service_ip)}


@app.route('/api/net/subnet', methods=['GET'])
def subnet_request():
    """
    Returns a new subnetwork address
    """
    addr = new_subnetwork_addr()
    return {'subnet_addr': addr}


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
