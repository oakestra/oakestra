import os
import requests
import time

NET_PLUGIN_ADDR = 'http://' + os.environ.get('NET_PLUGIN_URL') + ':' + str(os.environ.get('NET_PLUGIN_PORT'))


def net_inform_deploy(job, job_id):
    """
    Inform the network plugin about the deploy
    """
    print('new job: communicating deploy to netowkr plugin...')
    print(job)
    request_addr = SCHUEDULER_ADDR + '/api/service/deploy'
    print(request_addr)
    try:
        requests.post(request_addr, json={'deployment_descriptor': job, 'system_job_id': job_id})
    except requests.exceptions.RequestException as e:
        print('Calling Cloud Scheduler /api/calculate/deploy not successful.')
