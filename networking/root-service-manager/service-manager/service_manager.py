import os
from flask import Flask, flash, request, jsonify
from flask_socketio import SocketIO, emit
import json
from bson.objectid import ObjectId
from markupsafe import escape
import time
import threading
from bson import json_util
from network_management import new_instance_ip, clear_instance_ip, service_resolution, new_subnetwork_addr, \
    service_resolution_ip, new_job_rr_address
from mongodb_client import *
from net_logging import configure_logging

my_logger = configure_logging()

app = Flask(__name__)
app.secret_key = b'\xc8I\xae\x85\x90E\x9aBxQP\xde\x8es\xfdY'
app.logger.addHandler(my_logger)

socketio = SocketIO(app, async_mode='eventlet', logger=True, engineio_logger=True, cors_allowed_origins='*')

MY_PORT = os.environ.get('MY_PORT') or 10100


# ......... Deployment Endpoints .......................#
# ......................................................#

@app.route('/api/net/service/net_deploy_status', methods=['POST'])
def get_cluster_deployment_status_feedback():
    """
    Result of the deploy operation in a cluster and the subsequent generated network addresses
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

    app.logger.info("Incoming Request /api/job/net_deploy_status")
    data = request.json
    app.logger.info(data)

    mongo_update_job_net_status(
        job_id=data.get('job_id'),
        instances=data.get('instances')
    )

    return "roger that"


@app.route('/api/net/service/deploy', methods=['POST'])
def new_service_deployment():
    """
    Input:
        {
            system_job_id:int,
            deployment_descriptor:{}
        }
    service deployment descriptor and job_id
    The System Manager decorates the service with a new RR Ip in its own DB
    """

    app.logger.info("Incoming Request /api/net/service/deploy")
    data = request.json
    app.logger.info(data)

    s_ip = [{
        "IpType": 'RR',
        "Address": new_job_rr_address(data.get("deployment_descriptor")),
    }]

    job_id = mongo_insert_job(
        {
            'system_job_id': data.get("system_job_id"),
            'deployment_descriptor': data.get("deployment_descriptor"),
            'service_ip_list': s_ip
        })

    return "roger that"


@app.route('/api/net/instance/deploy', methods=['POST'])
def new_instance_deployment():
    """
    Input:
        {
            system_job_id:int,
            replicas:int,
            cluster_id:string,
        }
    The System Manager adds an instance ip for a new deployed instance to a new cluster
    """

    app.logger.info("Incoming Request /api/net/instance/deploy")
    data = request.json
    app.logger.info(data)

    instance_list = []
    for i in range(data.get('replicas')):
        instance_info = {
            'instance_number': i,  # number generation must be changed when scale up and down ops are implemented
            'instance_ip': new_instance_ip(),
            'cluster_id': str(data.get('cluster_id')),
        }
        instance_list.append(instance_info)

    mongo_update_job_status_and_instances_by_system_job_id(
        system_job_id=data.get('system_job_id'),
        status='CLUSTER_SCHEDULED',
        replicas=data.get('replicas'),
        instance_list=instance_list
    )

    return "roger that"


# .............. Table query Endpoints .................#
# ......................................................#

@app.route('/api/net/service/<service_name>/instances', methods=['GET'])
def table_query_resolution_by_jobname(service_name):
    """
    Get all the instances of a job given the complete name
    """
    service_name = service_name.replace("_", ".")
    app.logger.info("Incoming Request /api/job/" + str(service_name) + "/instances")
    instance, siplist = service_resolution(service_name)
    return {'instance_list': instance, 'service_ip_list': siplist}


@app.route('/api/net/service/ip/<service_ip>/instances', methods=['GET'])
def table_query_resolution_by_ip(service_ip):
    """
    Get all the instances of a job given a Service IP in 172_30_x_y notation
    """
    service_ip = service_ip.replace("_", ".")
    app.logger.info("Incoming Request /api/job/ip/" + str(service_ip) + "/instances")
    return {'instance_list': service_resolution_ip(service_ip)}


# ........ Subnetwork management endpoints .............#
# ......................................................#

@app.route('/api/net/subnet', methods=['GET'])
def subnet_request():
    """
    Returns a new subnetwork address
    """
    addr = new_subnetwork_addr()
    return {'subnet_addr': addr}


if __name__ == '__main__':
    import eventlet

    mongo_init(app)
    eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)
