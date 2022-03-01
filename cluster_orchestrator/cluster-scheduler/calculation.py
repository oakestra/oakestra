import logging
import time

from mongodb_client import mongo_find_one_node, mongo_find_all_active_nodes, mongo_find_node_by_name


def calculate(app, job):
    print('calculating...')
    app.logger.info('calculating')

    # check here if job has any user preferences, e.g. on a specific node, a specific cpu architecture,

    if job.get('node'):
        return deploy_on_desired_node(job=job)
    else:
        return greedy_load_balanced_algorithm(job=job)


def first_fit_algorithm(job):
    """Which of the worker nodes fits the Qos of the deployment file as the first"""
    active_nodes = mongo_find_all_active_nodes()

    print('active_nodes: ')
    for node in active_nodes:

        try:
            available_cpu = node.get('current_cpu_cores_free')
            available_memory = node.get('current_free_memory_in_MB')
            node_info = node.get('node_info')
            technology = node_info.get('technology')

            job_req = job.get('requirements')
            if available_cpu >= job_req.get('cpu') and available_memory >= job_req.get('memory') and job.get(
                    'image_runtime') in technology:
                return 'positive', node
        except:
            logging.error("Something wrong with job requirements or node infos")

    # no node found
    return 'negative', 'FAILED_NoCapacity'


def deploy_on_desired_node(job):
    job_req = job.get('requirements')
    desired_node = job_req.get('node')
    node = mongo_find_node_by_name(desired_node)

    if node is None or node == "Error":
        return 'negative', 'FAILED_Node_Not_Found'

    available_cpu = node.get('current_cpu_cores_free')
    available_memory = node.get('current_free_memory_in_MB')

    if available_cpu >= job_req.get('cpu') and available_memory >= job_req.get('memory'):
        return "positive", node

    return 'negative', 'FAILED_Desired_Node_No_Capacity'


def greedy_load_balanced_algorithm(job):
    """Which of the nodes within the cluster have the most capacity for a given job"""

    job_req = job.get('requirements')

    active_nodes = mongo_find_all_active_nodes()
    qualified_nodes = []

    for node in active_nodes:
        print(node)
        available_cpu = float(node.get('current_cpu_cores_free'))
        available_memory = float(node.get('current_free_memory_in_MB'))
        node_info = node.get('node_info')
        technology = node_info.get('technology')

        if available_cpu >= float(job_req.get('cpu')) \
                and available_memory >= int(job_req.get('memory')) \
                and job.get('image_runtime') in technology:
            qualified_nodes.append(node)

    target_node = None
    target_cpu = 0
    target_mem = 0

    # return if no qualified clusters found
    if len(qualified_nodes) < 1:
        return 'negative', 'NoActiveClusterWithCapacity'

    # return the cluster with the most cpu+ram
    for node in qualified_nodes:
        cpu = float(node.get('current_cpu_cores_free'))
        mem = float(node.get('current_free_memory_in_MB'))
        if cpu >= target_cpu and mem >= target_mem:
            target_cpu = cpu
            target_mem = target_cpu
            target_node = node

    return 'positive', target_node


def replicate(job):
    return 1
