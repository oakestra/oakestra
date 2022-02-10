import os
import requests
import time

NET_PLUGIN_ADDR = 'http://' + os.environ.get('NET_PLUGIN_URL') + ':' + str(os.environ.get('NET_PLUGIN_PORT'))


def net_inform_service_deploy(job, job_id):
    """
    Inform the network plugin about the deploy
    """
    print('new job: communicating service deploy to netowkr plugin...')
    print(job)
    request_addr = NET_PLUGIN_ADDR + '/api/net/service/deploy'
    print(request_addr)
    try:
        requests.post(request_addr, json={'deployment_descriptor': job, 'system_job_id': job_id})
    except requests.exceptions.RequestException as e:
        print('Calling network plugin /api/net/service/deploy not successful.')


def net_inform_instance_deploy(job_id, replicas, cluster_id):
    """
    Inform the network plugin about the new service's instance scheduled
    """
    print('new job: communicating instance deploy to network plugin...')
    print(replicas)
    request_addr = NET_PLUGIN_ADDR + '/api/net/instance/deploy'
    print(request_addr)
    try:
        requests.post(request_addr, json={'replicas': replicas, 'cluster_id': cluster_id, 'system_job_id': job_id})
    except requests.exceptions.RequestException as e:
        print('Calling network plugin /api/net/instance/deploy not successful.')
