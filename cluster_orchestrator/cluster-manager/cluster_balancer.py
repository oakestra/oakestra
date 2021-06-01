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

    returns:
        name: string #service name
        instances: {
                        instance_number: int
                        namespace_ip: string
                        host_ip: string
                        host_port: string
                        service_ip: [
                            {
                                IpType: string
                                Address: string
                            }
                        ]
                    }
    """
    # resolve it locally
    job = mongo_find_job_by_ip(ip_string)

    # if no results, ask the root orc
    if job is None:
        job = cloud_table_query_ip(ip_string)
        if job is None:
            return "", []

    instances = job['instance_list']
    service_ip_list = job['service_ip_list']
    for elem in instances:
        elem['service_ip'] = service_ip_list
        elem['service_ip'].append({
            "IpType": "instance_ip",
            "Address": elem['instance_ip']
        })

    name = job.get('job_name')

    return name, instances
