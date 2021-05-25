import time
import requests


def cluster_request_to_deploy(cluster_obj, job):
    print('propagate to cluster...')
    # cluster = mongo_find_one_cluster() # for debug purposes: take a cluster from database
    print(cluster_obj)
    cluster_addr = 'http://' + cluster_obj.get('ip') + ':' + str(cluster_obj.get('port')) + '/api/deploy'
    try:
        job['_id'] = str(job['_id'])
        resp = requests.post(cluster_addr, json=job)
        print(resp)
    except requests.exceptions.RequestException as e:
        print('Calling Cluster Orchestrator /api/deploy not successful.')


def cluster_request_to_delete_job(cluster_obj, job_id):
    cluster_addr = 'http://' + cluster_obj.get('ip') + ':' + str(cluster_obj.get('port')) + '/api/delete/' + job_id
    try:
        resp = requests.get(cluster_addr)
        print(resp)
    except requests.exceptions.RequestException as e:
        print('Calling Cluster Orchestrator /api/delete not successful.')


def cluster_request_to_replicate_up(cluster_obj, job_obj, int_replicas):
    cluster_addr = 'http://' + cluster_obj.get('ip') + ':' + str(cluster_obj.get('port')) + '/api/replicate/'
    try:
        resp = requests.post(cluster_addr, json={'job': job_obj, 'int_replicas': int_replicas})
        print(resp)
        return 1
    except requests.exceptions.RequestException as e:
        print('Calling Cluster Orchestrator /api/replicate not successful.')


def cluster_request_to_replicate_down(cluster_obj, job_obj, int_replicas):
    cluster_addr = 'http://' + cluster_obj.get('ip') + ':' + str(cluster_obj.get('port')) + '/api/replicate/'
    try:
        resp = requests.post(cluster_addr, json={'job': job_obj, 'int_replicas': int_replicas})
        print(resp)
        return 1
    except requests.exceptions.RequestException as e:
        print('Calling Cluster Orchestrator /api/replicate not successful.')


def cluster_request_to_move_within_cluster(cluster_obj, job_id, node_from, node_to):
    cluster_addr = 'http://' + cluster_obj.get('ip') + ':' + str(cluster_obj.get('port')) + '/api/move/'
    try:
        resp = requests.post(cluster_addr, json={'job': job_id, 'node_from': node_from, 'node_to': node_to})
        print(resp)
        return 1
    except requests.exceptions.RequestException as e:
        print('Calling Cluster Orchestrator /api/move not successful.')
