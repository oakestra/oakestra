import json
import requests

NET_MANAGER_ADDR = 'http://localhost:10010'


def net_manager_docker_deploy(job,containerid):
    print('Asking for network deploy to the network manager component ')
    print(containerid)
    request_address = NET_MANAGER_ADDR + '/docker/deploy'

    request = {
        'containerId': containerid,
        'serviceName': job['job_name'],
        'instanceNumber': 0,
    }

    print(request)

    try:
        response = requests.post(request_address, json=request)
        if response.status_code == 200:
            print(response.text)
            response = json.loads(response.text)
        else:
            raise Exception("Error during netcall, code: "+str(response.status_code))
        return response.get('nsAddress')
    except requests.exceptions.RequestException as e:
        print('Calling NetManager not successful.')


def net_manager_docker_undeploy(containerid):
    print('Asking for network undeploy to the network manager component ')
    print(containerid)
    request_address = NET_MANAGER_ADDR + '/docker/undeploy'

    request = {
        'serviceName': containerid
    }
    print(request)

    try:
        requests.post(request_address, json=request)
    except requests.exceptions.RequestException as e:
        print('Calling NetManager not successful.')


def net_manager_register(client_id):
    print('Initializing the NetManager')
    print(client_id)
    request_address = NET_MANAGER_ADDR + '/register'

    request = {
        'client_id': client_id
    }
    print(request)

    try:
        requests.post(request_address, json=request)
    except requests.exceptions.RequestException as e:
        print('Calling NetManager not successful.')