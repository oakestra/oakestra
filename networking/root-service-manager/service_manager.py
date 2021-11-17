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
from cluster_requests import *
from scheduler_requests import scheduler_request_deploy, scheduler_request_replicate, scheduler_request_status
from sm_logging import configure_logging

my_logger = configure_logging()

app = Flask(__name__)
app.secret_key = b'\xc8I\xae\x85\x90E\x9aBxQP\xde\x8es\xfdY'

socketio = SocketIO(app, async_mode='eventlet', logger=True, engineio_logger=True, cors_allowed_origins='*')

mongo_init(app)

MY_PORT = os.environ.get('CLUSTER_SERVICE_MANAGER_PORT') or 10034


# ......... Deployment Endpoints .......................#
# ......................................................#

@app.route('/api/job/net_deploy_status', methods=['POST'])
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

@app.route('/api/service/deploy', methods=['POST'])
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

    app.logger.info("Incoming Request /api/job/net_deploy_status")
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





if __name__ == '__main__':
    # start_http_server(10008)

    # socketio.run(app, debug=True, host='0.0.0.0', port=MY_PORT)
    import eventlet

    eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)
