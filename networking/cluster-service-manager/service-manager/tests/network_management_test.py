import json
import time
from unittest.mock import MagicMock, patch
from unittest import mock
import sys
from network import deployment
from network.tablequery import resolution, interests
from interfaces.mqtt_client import _tablequery_handler
from interfaces import mqtt_client

mongodb_client = sys.modules['interfaces.mongodb_requests']


def _get_fake_job(name):
    return {
        "job_name": name,
        "system_job_id": "123",
        "instance_list": [
            {
                "worker_id": "abab",
                "instance_number": 0,
                "namespace_ip": "0.0.0.1",
                "instance_ip": "172.30.0.2",
                "host_ip": "0.0.0.0",
                "host_port": "5000"
            }
        ],
        "service_ip_list": [
            {
                "IpType": "RR",
                "Address": "172.30.0.1"
            }
        ]
    }


def test_deployment_status_report(requests_mock):
    from interfaces.root_service_manager_requests import ROOT_SERVICE_MANAGER_ADDR
    job = _get_fake_job("aaa")
    mongodb_client.mongo_update_job_deployed = MagicMock()
    mongodb_client.mongo_find_job_by_name = MagicMock(return_value=job)
    adapter = requests_mock.post(ROOT_SERVICE_MANAGER_ADDR + "/api/net/service/net_deploy_status", status_code=200)
    job_instance = job['instance_list'][0]

    deployment.deployment_status_report(
        "aaa",
        "DEPLOYED",
        job_instance['namespace_ip'],
        job_instance['worker_id'],
        job_instance['instance_number'],
        job_instance['host_ip'],
        job_instance['host_port']
    )

    mongodb_client.mongo_update_job_deployed.assert_called_with("aaa", "DEPLOYED", "0.0.0.1", "abab", 0, "0.0.0.0",
                                                                "5000")
    instances = [{
        'instance_number': job_instance['instance_number'],
        'namespace_ip': job_instance['namespace_ip'],
        'host_ip': job_instance['host_ip'],
        'host_port': job_instance['host_port']
    }]
    data = {
        'job_id': job["system_job_id"],
        'instances': instances
    }
    assert adapter.call_count == 1
    assert adapter.called
    assert adapter.last_request.json() == data


@patch('network.tablequery.interests.add_interest')
def test_tablequery_service_ip_local(add_interest):
    job = _get_fake_job("aaa")
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=job)
    mqtt_client.mqtt_publish_tablequery_result = MagicMock()
    job_instance = job['instance_list'][0]

    _tablequery_handler("baba", {
        'sip': '172.30.0.1'
    })

    add_interest.assert_called_with("aaa", "baba")
    job_instance['service_ip'] = [
        {
            "IpType": "instance_ip",
            "Address": job_instance['instance_ip'],
        },
        {
            "IpType": "RR",
            "Address": job['service_ip_list'][0]["Address"],
        }
    ]
    mqtt_client.mqtt_publish_tablequery_result.assert_called_with("baba", {
        'app_name': "aaa",
        'instance_list': [job_instance]
    })


@patch('network.tablequery.interests.add_interest')
def test_tablequery_service_name_local(add_interest):
    job = _get_fake_job("aaa")
    mongodb_client.mongo_find_job_by_name = MagicMock(return_value=job.copy())
    mqtt_client.mqtt_publish_tablequery_result = MagicMock()
    job_instance = job['instance_list'][0]

    _tablequery_handler("baba", {
        'sname': 'aaa'
    })

    add_interest.assert_called_with("aaa", "baba")
    job_instance['service_ip'] = [
        {
            "IpType": "instance_ip",
            "Address": job_instance['instance_ip'],
        },
        {
            "IpType": "RR",
            "Address": job['service_ip_list'][0]["Address"],
        }
    ]
    mqtt_client.mqtt_publish_tablequery_result.assert_called_with("baba", {
        'app_name': "aaa",
        'instance_list': [job_instance]
    })


@patch('network.tablequery.interests.add_interest')
def test_tablequery_service_ip_cloud(add_interest, requests_mock):
    from interfaces.root_service_manager_requests import ROOT_SERVICE_MANAGER_ADDR
    job = _get_fake_job("aaa")
    job_instance = job['instance_list'][0]
    adapter = requests_mock.get(
        ROOT_SERVICE_MANAGER_ADDR +
        "/api/net/service/ip/" + job['service_ip_list'][0]["Address"].replace(".", "_") + "/instances",
        status_code=200, json=dict(job)
    )
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)
    mongodb_client.mongo_insert_job = MagicMock()
    mqtt_client.mqtt_publish_tablequery_result = MagicMock()

    _tablequery_handler("baba", {
        'sip': '172.30.0.1'
    })

    mongodb_client.mongo_insert_job.assert_called_with(dict(job))
    add_interest.assert_called_with("aaa", "baba")
    job_instance['service_ip'] = [
        {
            "IpType": "RR",
            "Address": job['service_ip_list'][0]["Address"],
        },
        {
            "IpType": "instance_ip",
            "Address": job_instance['instance_ip'],
        }
    ]
    mqtt_client.mqtt_publish_tablequery_result.assert_called_with("baba", {
        'app_name': "aaa",
        'instance_list': [job_instance]
    })

@patch('network.tablequery.interests.add_interest')
def test_tablequery_service_name_cloud(add_interest,requests_mock):
    from interfaces.root_service_manager_requests import ROOT_SERVICE_MANAGER_ADDR
    job = _get_fake_job("aaa")
    job_instance = job['instance_list'][0]
    adapter = requests_mock.get(
        ROOT_SERVICE_MANAGER_ADDR +
        "/api/net/service/" + "aaa" + "/instances",
        status_code=200, json=dict(job)
    )
    mongodb_client.mongo_find_job_by_name = MagicMock(return_value=None)
    mongodb_client.mongo_insert_job = MagicMock()
    mqtt_client.mqtt_publish_tablequery_result = MagicMock()

    _tablequery_handler("baba", {
        'sname': 'aaa'
    })

    mongodb_client.mongo_insert_job.assert_called_with(dict(job))
    add_interest.assert_called_with("aaa", "baba")
    job_instance['service_ip'] = [
        {
            "IpType": "RR",
            "Address": job['service_ip_list'][0]["Address"],
        },
        {
            "IpType": "instance_ip",
            "Address": job_instance['instance_ip'],
        }
    ]
    mqtt_client.mqtt_publish_tablequery_result.assert_called_with("baba", {
        'app_name': "aaa",
        'instance_list': [job_instance]
    })


def test_register_interest():
    # test worker not yet interested
    mongodb_client.mongo_get_interest_workers = MagicMock(return_value=["aaa","bbb"])
    mongodb_client.mongo_add_interest = MagicMock()
    interests.add_interest("app1.aa","dafsdòf22")
    mongodb_client.mongo_add_interest.assert_called_with("app1.aa","dafsdòf22")

    # test worker already interested
    mongodb_client.mongo_get_interest_workers = MagicMock(return_value=["dafsdòf22"])
    mongodb_client.mongo_add_interest = MagicMock()
    interests.add_interest("app1.aa", "dafsdòf22")
    mongodb_client.mongo_add_interest.assert_not_called()


def test_remove_interest(requests_mock):
    from interfaces.root_service_manager_requests import ROOT_SERVICE_MANAGER_ADDR

    # test interested workers >0
    adapter = requests_mock.register_uri(
        'DELETE',
        ROOT_SERVICE_MANAGER_ADDR +
        "/api/net/interest/" + "app1.aa",
        status_code=200
    )
    mongodb_client.mongo_remove_interest = MagicMock()
    mongodb_client.mongo_get_interest_workers = MagicMock(return_value=["aaa","bbb"])
    interests.remove_interest("app1.aa", "dafsdòf22")
    mongodb_client.mongo_remove_interest.assert_called_with("app1.aa", "dafsdòf22")
    assert adapter.call_count == 0


    # test interested workers ==0
    mongodb_client.mongo_remove_interest = MagicMock()
    mongodb_client.mongo_get_interest_workers = MagicMock(return_value=[])
    interests.remove_interest("app1.aa", "dafsdòf22")
    mongodb_client.mongo_remove_interest.assert_called_with("app1.aa", "dafsdòf22")
    assert adapter.call_count == 1
