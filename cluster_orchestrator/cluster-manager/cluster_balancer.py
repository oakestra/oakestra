from mongodb_client import mongo_find_job_by_ip, mongo_find_job_by_name
from system_manager_requests import cloud_table_query_ip, cloud_table_query_service_name


def service_resolution(service_name):
    """
    Resolves the service instance list by service name with the local DB,
    if no result found the query is propagated to the System Manager
    """
    # resolve it locally
    instances = mongo_find_job_by_name(service_name)
    # if no results, ask the root orc
    if instances is None:
        instances = cloud_table_query_service_name(service_name)
        instances = instances['instance_list']
    else:
        instances = instances['instance_list']

    return instances


def service_resolution_ip(ip_string):
    """
    Resolves the service instance list by service ServiceIP with the local DB,
    if no result found the query is propagated to the System Manager
    """
    # resolve it locally
    instances = mongo_find_job_by_ip(ip_string)
    # if no results, ask the root orc
    if instances is None:
        instances = cloud_table_query_ip(ip_string)
        instances = instances['instance_list']
    else:
        instances=instances['instance_list']

    return instances
