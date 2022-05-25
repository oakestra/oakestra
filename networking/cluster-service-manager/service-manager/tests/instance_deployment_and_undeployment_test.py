import json
import time
from unittest.mock import MagicMock
from unittest import mock
import sys
from interfaces import mqtt_client
from operations.instances_management import _update_cache_and_workers

mongodb_client = sys.modules['interfaces.mongodb_requests']


def _get_fake_job(name):
    return {
        "job_name": name,
        "system_job_id": "123",
        "instance_list": [
            {
                "instance_number": 0,
                "ip": "aaaaa",
                "status": "ACTIVE"
            }
        ],
        "service_ip_list": [
            {
                "type": "RR"
            }
        ]
    }


def test_instance_deployment_update(requests_mock):
    import interfaces.root_service_manager_requests as root_reqs
    root_reqs.ROOT_SERVICE_MANAGER_ADDR = "http://0.0.0.0:5000"
    job = _get_fake_job("aaa")
    mongodb_client.mongo_update_job_instance = MagicMock(return_value=job)
    mongodb_client.mongo_insert_job = MagicMock()
    mqtt_client.mqtt_notify_service_change = MagicMock()
    req_addr = root_reqs.ROOT_SERVICE_MANAGER_ADDR + "/api/net/service/" + job["job_name"] + "/instances"
    requests_mock.get(req_addr, json=job, status_code=200)

    _update_cache_and_workers("aaa", 0, "DEPLOYMENT")

    mongodb_client.mongo_update_job_instance.assert_called_with(job_name="aaa", instance=job["instance_list"][0])
    mqtt_client.mqtt_notify_service_change.assert_called_with(job_name="aaa", type="DEPLOYMENT")


def test_instance_undeployment_update():
    mongodb_client.mongo_remove_job_instance = MagicMock()
    mongodb_client.mongo_insert_job = MagicMock()
    mqtt_client.mqtt_notify_service_change = MagicMock()

    _update_cache_and_workers("aaa", 0, "UNDEPLOYMENT")

    mongodb_client.mongo_remove_job_instance.assert_called_with(job_name="aaa", instance_number=0)
    mqtt_client.mqtt_notify_service_change.assert_called_with(job_name="aaa", type="UNDEPLOYMENT")
