import requests
import os
import json

SYSTEM_MANAGER_ADDR = 'http://' + os.environ.get('SYSTEM_MANAGER_URL') + ':' + os.environ.get('SYSTEM_MANAGER_PORT')

def root_service_manager_get_subnet():
    print('Asking the System Manager for a subnet')
    try:
        response = requests.get(SYSTEM_MANAGER_ADDR + '/api/net/subnet')
        addr = json.loads(response.text).get('subnet_addr')
        if len(addr) > 0:
            return addr
        else:
            raise requests.exceptions.RequestException('No address found')
    except requests.exceptions.RequestException as e:
        print('Calling System Manager /api/information not successful.')


def system_manager_notify_deployment_status(job, worker_id):
    print('Sending deployment status information to System Manager.')
    data = {
        'job_id': job.get('system_job_id'),
        'instances': [],
    }
    # prepare json data information
    for instance in job['instance_list']:
        if instance['worker_id'] == worker_id:
            elem = {
                'instance_number': instance['instance_number'],
                'namespace_ip': instance['namespace_ip'],
                'host_ip': instance['host_ip'],
                'host_port': instance['host_port'],
            }
            data['instances'].append(elem)
    try:
        requests.post(SYSTEM_MANAGER_ADDR + '/api/result/cluster_deploy', json=data)
    except requests.exceptions.RequestException as e:
        print('Calling System Manager /api/result/cluster_deploy not successful.')


def cloud_table_query_ip(ip):
    print('table query to the System Manager...')
    job_ip = ip.replace(".", "_")
    request_addr = SYSTEM_MANAGER_ADDR + '/api/job/ip/' + str(job_ip) + '/instances'
    print(request_addr)
    try:
        return requests.get(request_addr).json()
    except requests.exceptions.RequestException as e:
        print('Calling System Manager /api/job/ip/../instances not successful.')


def cloud_table_query_service_name(name):
    print('table query to the System Manager...')
    job_name = name.replace(".", "_")
    request_addr = SYSTEM_MANAGER_ADDR + '/api/job/' + str(job_name) + '/instances'
    print(request_addr)
    try:
        return requests.get(request_addr).json()
    except requests.exceptions.RequestException as e:
        print('Calling System Manager /api/job/../instances not successful.')