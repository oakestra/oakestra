import requests
import threading
import os

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
