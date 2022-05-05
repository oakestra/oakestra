import logging
import os
import requests
import time

NET_PLUGIN_ADDR = 'http://' + os.environ.get('NET_PLUGIN_URL','localhost') + ':' + str(os.environ.get('NET_PLUGIN_PORT','10010'))


def net_inform_service_deploy(job, job_id):
    """
    Inform the network plugin about the deploy
    """
    logging.debug('new job: communicating service deploy to netowkr plugin...')
    logging.debug(job)
    request_addr = NET_PLUGIN_ADDR + '/api/net/service/deploy'
    logging.debug(request_addr)
    try:
        r = requests.post(request_addr, json={'deployment_descriptor': job, 'system_job_id': job_id})
        r.raise_for_status()
    except requests.exceptions.ConnectionError as errc:
        logging.error('Calling network plugin ' + request_addr + ' Connection error.')
    except requests.exceptions.Timeout as errt:
        logging.error('Calling network plugin ' + request_addr + ' Timeout error.')
    except requests.exceptions.RequestException as err:
        logging.error('Calling network plugin ' + request_addr + ' not successful.')


def net_inform_instance_deploy(job_id, replicas, cluster_id):
    """
    Inform the network plugin about the new service's instance scheduled
    """
    logging.debug('new job: communicating instance deploy to network plugin...')
    logging.debug(replicas)
    request_addr = NET_PLUGIN_ADDR + '/api/net/instance/deploy'
    logging.debug(request_addr)
    try:
        r = requests.post(request_addr, json={'replicas': replicas, 'cluster_id': cluster_id, 'system_job_id': job_id})
        r.raise_for_status()
    except requests.exceptions.ConnectionError as errc:
        logging.error('Calling network plugin ' + request_addr + ' Connection error.')
    except requests.exceptions.Timeout as errt:
        logging.error('Calling network plugin ' + request_addr + ' Timeout error.')
    except requests.exceptions.RequestException as err:
        logging.error('Calling network plugin ' + request_addr + ' Request Exception.')


def net_inform_instance_undeploy(job_id, instance):
    """
    Inform the network plugin about an undeployed instance
    """
    logging.debug('new job: communicating instance undeploy to network plugin...')
    request_addr = NET_PLUGIN_ADDR + '/api/net/' + str(job_id) + '/' + str(instance)
    logging.debug(request_addr)
    try:
        r = requests.delete(request_addr)
        r.raise_for_status()
    except requests.exceptions.ConnectionError as errc:
        logging.error('Calling network plugin ' + request_addr + ' Connection error.')
    except requests.exceptions.Timeout as errt:
        logging.error('Calling network plugin ' + request_addr + ' Timeout error.')
    except requests.exceptions.RequestException as err:
        logging.error('Calling network plugin ' + request_addr + ' Request Exception.')


def net_register_cluster(cluster_id, cluster_address, cluster_port):
    """
    Inform the network plugin about the new registered cluster
    """
    logging.debug('new job: communicating cluster registration to net component...')
    request_addr = NET_PLUGIN_ADDR + '/api/net/cluster'
    try:
        r = requests.post(request_addr,
                          json={
                              'cluster_id': cluster_id,
                              'cluster_address': cluster_address,
                              'cluster_port': cluster_port
                          })
        logging.debug(r)
        r.raise_for_status()
    except requests.exceptions.ConnectionError as errc:
        logging.error('Calling network plugin ' + request_addr + ' Connection error.')
    except requests.exceptions.Timeout as errt:
        logging.error('Calling network plugin ' + request_addr + ' Timeout error.')
    except requests.exceptions.RequestException as err:
        logging.error('Calling network plugin ' + request_addr + ' Request Exception.')
