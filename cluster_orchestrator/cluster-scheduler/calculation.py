import time

from mongodb_client import mongo_find_one_node, mongo_find_all_active_nodes, mongo_find_node_by_name


def calculate(app, job):
    print('calculating...')
    app.logger.info('calculating')

    # check here if job has any user preferences, e.g. on a specific node, a specific cpu architecture,

    if job.get('node'):
        return deploy_on_desired_node(job=job)
    else:
        return first_fit_algorithm(job=job)


def first_fit_algorithm(job):
    """Which of the worker nodes fits the Qos of the deployment file as the first"""
    active_nodes = mongo_find_all_active_nodes()

    print('active_nodes: ')
    for node in active_nodes:
        print(node)

        available_cpu = node.get('current_cpu_cores_free')
        available_memory = node.get('current_free_memory_in_MB')
        node_info = node.get('node_info')
        technology = node_info.get('technology')

        job_req = job.get('requirements')
        if available_cpu >= job_req.get('cpu') and available_memory >= job_req.get('memory') and job.get('image_runtime') in technology:
            return 'positive', node

    # no node found
    return 'negative', 'FAILED_NoCapacity'


def deploy_on_desired_node(job):

    job_req = job.get('requirements')
    desired_node = job_req.get('node')
    node = mongo_find_node_by_name(desired_node)

    if node == 'Error':
        return 'negative', 'FAILED_Node_Not_Found'

    available_cpu = node.get('current_cpu_cores_free')
    available_memory = node.get('current_free_memory_in_MB')

    if available_cpu >= job_req.get('cpu') and available_memory >= job_req.get('memory'):
        return "positive", node

    return 'negative', 'FAILED_Desired_Node_No_Capacity'


def replicate(job):
    return 1


