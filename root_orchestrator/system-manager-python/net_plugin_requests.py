import logging
import os
import requests
import time

NET_PLUGIN_ADDR = 'http://' + os.environ.get('NET_PLUGIN_URL') + ':' + str(os.environ.get('NET_PLUGIN_PORT'))


def net_inform_service_deploy(job, job_id):
    """
    Inform the network plugin about the deploy
    """
    logging.debug('new job: communicating service deploy to netowkr plugin...')
    logging.debug(job)
    request_addr = NET_PLUGIN_ADDR + '/api/net/service/deploy'
    logging.debug(request_addr)
    try:
        requests.post(request_addr, json={'deployment_descriptor': job, 'system_job_id': job_id})
    except requests.exceptions.RequestException as e:
        logging.error('Calling network plugin /api/net/service/deploy not successful.')


def net_inform_instance_deploy(job_id, replicas, cluster_id):
    """
    Inform the network plugin about the new service's instance scheduled
    """
    logging.debug('new job: communicating instance deploy to network plugin...')
    logging.debug(replicas)
    request_addr = NET_PLUGIN_ADDR + '/api/net/instance/deploy'
    logging.debug(request_addr)
    try:
        requests.post(request_addr, json={'replicas': replicas, 'cluster_id': cluster_id, 'system_job_id': job_id})
    except requests.exceptions.RequestException as e:
        logging.error('Calling network plugin /api/net/instance/deploy not successful.')


def net_inform_instance_undeploy(job_id, instance):
    """
    Inform the network plugin about an undeployed instance
    """
    logging.debug('new job: communicating instance undeploy to network plugin...')
    request_addr = NET_PLUGIN_ADDR + '/api/net/'+str(job_id)+'/'+str(instance)
    logging.debug(request_addr)
    try:
        requests.delete(request_addr)
    except requests.exceptions.RequestException as e:
        logging.error('Calling network plugin /api/net/instance/deploy not successful.')


def net_register_cluster(cluster_id, cluster_address, cluster_port):
    """
    Inform the network plugin about the new registered cluster
    """
    logging.debug('new job: communicating cluster registration to net component...')
    request_addr = NET_PLUGIN_ADDR + '/api/net/cluster'
    try:
        req = requests.post(request_addr,
                      json={
                          'cluster_id': cluster_id,
                          'cluster_address': cluster_address,
                          'cluster_port': cluster_port
                      })
        logging.debug(req)
    except requests.exceptions.RequestException as e:
        logging.error('Calling network plugin /api/net/cluster not successful.')
