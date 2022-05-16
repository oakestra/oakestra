from unittest.mock import MagicMock
import operations.instances_management
import operations.cluster_management
import interfaces.clusters_interface
import sys

mongodb_client = sys.modules['interfaces.mongodb_requests']


def _get_fake_job(name):
    return {
        "job_name": name,
        "system_job_id": "123",
        "instance_list": [
            {
                "instance_number": 1
            }
        ],
        "service_ip_list": [
            {
                "type": "RR"
            }
        ]
    }


def _get_fake_cluster():
    return {
        "_id": "1",
        "cluster_id": "1",
        "cluster_address": "192.168.1.1",
        "cluster_port": "5555",
        "status": operations.cluster_management.CLUSTER_STATUS_ACTIVE,
        "instances": ["aaa", "bbb"],
    }


def test_interest_register():
    fake_cluster = _get_fake_cluster()
    fake_job = _get_fake_job("aaa")
    mongodb_client.mongo_get_cluster_by_ip = MagicMock(return_value=fake_cluster)
    mongodb_client.mongo_find_job_by_name = MagicMock(return_value=fake_job)
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=fake_job)
    mongodb_client.mongo_register_cluster_job_interest = MagicMock()

    result, code = operations.instances_management.get_service_instances(name="aaa", cluster_ip="123")

    assert code == 200
    assert result["instance_list"] is not None
    assert result["service_ip_list"] is not None
    assert result["system_job_id"] == "123"
    assert result["job_name"] == "aaa"

    mongodb_client. \
        mongo_register_cluster_job_interest. \
        assert_called_with(
        fake_cluster["cluster_id"],
        fake_job["job_name"]
    )
