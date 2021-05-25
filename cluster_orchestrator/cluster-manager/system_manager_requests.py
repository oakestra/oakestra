import requests
import threading
import os
import json

from mongodb_client import mongo_aggregate_node_information
from my_prometheus_client import prometheus_set_metrics

SYSTEM_MANAGER_ADDR = 'http://' + os.environ.get('SYSTEM_MANAGER_URL') + ':' + os.environ.get('SYSTEM_MANAGER_PORT')


def send_aggregated_info_to_sm(my_id, time_interval):
    data = mongo_aggregate_node_information(time_interval)
    threading.Thread(group=None, target=send_aggregated_info,
                     args=(my_id, data)).start()
    prometheus_set_metrics(my_id=my_id, data=data)


def send_aggregated_info(my_id, data):
    print('Sending aggregated information to System Manager.')
    try:
        requests.post(SYSTEM_MANAGER_ADDR + '/api/information/' + str(my_id), json=data)
    except requests.exceptions.RequestException as e:
        print('Calling System Manager /api/information not successful.')


def cloud_request_incr_node(my_id):
    print('reporting to cloud about new worker node...')
    request_addr = SYSTEM_MANAGER_ADDR + '/api/cluster/' + str(my_id) + '/incr_node'
    print(request_addr)
    try:
        requests.get(request_addr)
    except requests.exceptions.RequestException as e:
        print('Calling System Manager /api/cluster/../incr_node not successful.')


def system_manager_get_subnet():
    print('Asking the System Manager for a subnet')
    try:
        response = requests.get(SYSTEM_MANAGER_ADDR + '/api/net/subnet')
        addr = json.loads(response.text).get('subnet_addr')
        if len(addr) > 0:
            return addr
        else:
            raise requests.exceptions.RequestException('No address found')
    except requests.exceptions.RequestException as e:
        print('Calling System Manager /api/information not successful.')


def system_manager_notify_deployment_status(job, worker_id):
    print('Sending deployment status information to System Manager.')
    data = {
        'job_id': job.get('system_job_id'),
        'instances': [],
    }
    # prepare json data information
    for instance in job['instance_list']:
        if instance['worker_id'] is worker_id:
            elem = {
                'instance_number': instance['instance_number'],
                'namespace_ip': instance['namespace_ip'],
                'host_ip': instance['host_ip'],
                'host_port': instance['host_port'],
            }
            data['instances'].append(elem)
    try:
        requests.post(SYSTEM_MANAGER_ADDR + '/api/result/cluster_deploy', json=data)
    except requests.exceptions.RequestException as e:
        print('Calling System Manager /api/result/cluster_deploy not successful.')
