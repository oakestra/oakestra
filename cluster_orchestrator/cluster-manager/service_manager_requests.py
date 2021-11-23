import requests
import threading
import os
import json
import traceback

from mongodb_client import mongo_aggregate_node_information
from my_prometheus_client import prometheus_set_metrics

SERVICE_MANAGER_ADDR = 'http://' + os.environ.get('CLUSTER_SERVICE_MANAGER_ADDR') + ':' + os.environ.get(
    'CLUSTER_SERVICE_MANAGER_PORT')


def network_notify_deployment(job_id, job):
    print('Sending network deployment notification to the network component')
    try:
        requests.post(SERVICE_MANAGER_ADDR + '/api/net/deployment', json={'job_id': job_id, 'data': job})
    except requests.exceptions.RequestException as e:
        print('Calling Service Manager /api/net/deployment not successful.')


def network_notify_migration(job_id, job):
    pass


def network_notify_undeployment(job_id, job):
    pass
