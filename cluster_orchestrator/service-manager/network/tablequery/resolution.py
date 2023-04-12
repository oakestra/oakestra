from interfaces import mongodb_requests
from interfaces import root_service_manager_requests
import copy

from interfaces.mongodb_requests import mongo_update_job_instance


def service_resolution(service_name):
    """
    Resolves the service instance list by service name with the local DB,
    if no result found the query is propagated to the System Manager

    returns:
        instance_list: [{
                        instance_number: int
                        instance_ip: string
                        namespace_ip: string
                        host_ip: string
                        host_port: string
                        }]
        service_ip_list: [{
                            IpType: string
                            Address: string
                        }]
    """
    # resolve it locally
    job = mongodb_requests.mongo_find_job_by_name(service_name)
    instances = None
    siplist = None

    # if no results, ask the root orc
    if job is None:
        job = root_service_manager_requests.cloud_table_query_service_name(service_name)
        instances = job['instance_list']
        siplist = job['service_ip_list']
        mongodb_requests.mongo_insert_job(copy.deepcopy(job))
        for instance in instances:
            mongo_update_job_instance(job['job_name'], instance)
    else:
        instances = job['instance_list']
        siplist = job['service_ip_list']

    return instances, siplist


def service_resolution_ip(ip_string):
    """
    Resolves the service instance list by service ServiceIP with the local DB,
    if no result found the query is propagated to the System Manager

    returns:
        name: string #service name
        instance_list: [{
                        instance_number: int
                        namespace_ip: string
                        namespace_ip_v6: string
                        host_ip: string
                        host_port: string
                    }]
        service_ip_list: [{
                                IpType: string
                                Address: string
                                Address_v6: string
                    }]

    """
    # resolve it locally
    job = mongodb_requests.mongo_find_job_by_ip(ip_string)

    # if no results, ask the root orc
    if job is None:
        job = root_service_manager_requests.cloud_table_query_ip(ip_string)
        mongodb_requests.mongo_insert_job(copy.deepcopy(job))
        for instance in job.get('instance_list'):
            mongo_update_job_instance(job['job_name'], instance)
    return job.get("job_name"), job.get('instance_list'), job.get('service_ip_list')

# TODO TEST
def format_instance_response(instance_list, sip_list):
    for elem in instance_list:
        elem['service_ip'] = copy.deepcopy(sip_list)
        elem['service_ip'].append({
            "IpType": "instance_ip",
            "Address": elem['instance_ip'],
            "Address_v6": elem['instance_ip_v6']
        })
    return instance_list
