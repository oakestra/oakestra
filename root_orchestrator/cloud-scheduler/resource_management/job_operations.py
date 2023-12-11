import os
from requests import get, patch, exceptions

RESOURCE_ABSTRACTOR_ADDR = f"http://{os.environ.get('RESOURCE_ABSTRACTOR_URL')}:{os.environ.get('RESOURCE_ABSTRACTOR_PORT')}"
JOBS_API = f'{RESOURCE_ABSTRACTOR_ADDR}/api/v1/jobs'

def get_jobs(**kwargs):
    request_address = JOBS_API
    try:
        response = get(request_address, params=kwargs)
        return response.json()
    except exceptions.RequestException as e:
        print('Calling Resource Abstractor /api/v1/resources not successful.')
    
    return []

def get_job_by_id(job_id):
    request_address = f'{JOBS_API}/{job_id}'
    try:
        response = get(request_address)
        # TODO check body not empty.
        return response.json()
    except exceptions.RequestException as e:
        print(f'Calling Resource Abstractor /api/v1/resources/{job_id} not successful.')
    
    return None

def update_job_status(job_id, status):
    request_address = f'{JOBS_API}/{job_id}'
    try:
        response = patch(request_address, json={'status': status})
        return response
    except exceptions.RequestException as e:
        print(f'Calling Resource Abstractor /api/v1/resources/{job_id} not successful.')
    
    return None
