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


def test_deploy_request():
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 253])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)
    mongodb_client.mongo_create_job_instance = MagicMock()

    res, code = operations.instances_management.deploy_request("123", 0, "abc")

    assert code == 200
    mongodb_client. \
        mongo_create_job_instance. \
        assert_called_with(
        system_job_id="123",
        instance={
            "instance_number": 0,
            "instance_ip": "10.30.0.253",
            "cluster_id": "abc"
        }
    )

def test_deploy_request_2_instances():
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 253])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)
    mongodb_client.mongo_create_job_instance = MagicMock()

    res, code = operations.instances_management.deploy_request("123", 0, "abc")

    assert code == 200
    mongodb_client. \
        mongo_create_job_instance. \
        assert_called_with(
        system_job_id="123",
        instance={
            "instance_number": 0,
            "instance_ip": "10.30.0.253",
            "cluster_id": "abc"
        }
    )

    mongodb_client.mongo_update_next_service_ip.assert_called_with([10, 30, 1, 0])
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 1, 0])
    res, code = operations.instances_management.deploy_request("123", 1, "abc")

    assert code == 200
    mongodb_client. \
        mongo_create_job_instance. \
        assert_called_with(
        system_job_id="123",
        instance={
            "instance_number": 1,
            "instance_ip": "10.30.1.0",
            "cluster_id": "abc"
        }
    )

    mongodb_client.mongo_update_next_service_ip.assert_called_with([10, 30, 1, 1])
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 1, 1])
    res, code = operations.instances_management.deploy_request("123", 2, "abc")

    assert code == 200
    mongodb_client. \
        mongo_create_job_instance. \
        assert_called_with(
        system_job_id="123",
        instance={
            "instance_number": 2,
            "instance_ip": "10.30.1.1",
            "cluster_id": "abc"
        }
    )


def test_deploy_completed_cluster_notification():
    mongodb_client.mongo_update_job_net_status = MagicMock(return_value=_get_fake_job("aaa"))
    mongodb_client.mongo_get_cluster_interested_to_job = MagicMock(return_value=[_get_fake_cluster()])
    interfaces.clusters_interface.notify_deployment = MagicMock(return_value=200)

    instances = [{
        "instance_number": 0,
        "namespace_ip": "0.0.0.0",
        "host_ip": "0.0.0.0",
        "host_port": "5000"
    }]

    operations.instances_management.update_instance_local_addresses(job_id="123", instances=instances)

    interfaces.clusters_interface \
        .notify_deployment \
        .assert_called_with("192.168.1.1", "5555", "aaa", 0)

    mongodb_client \
        .mongo_update_job_net_status \
        .assert_called_with(job_id="123", instances=instances)



def test_undeploy_completed_cluster_notification():
    mongodb_client.mongo_find_job_by_systemid = MagicMock(return_value=_get_fake_job("aaa"))
    cluster = _get_fake_cluster()
    mongodb_client.mongo_get_cluster_interested_to_job = MagicMock(return_value=[cluster])
    interfaces.clusters_interface.notify_undeployment = MagicMock(return_value=200)

    operations.instances_management.undeploy_request(sys_job_id="123", instance_number=1)

    interfaces.clusters_interface \
        .notify_undeployment \
        .assert_called_with("192.168.1.1", "5555", "aaa", 1)

    mongodb_client \
        .mongo_find_job_by_systemid \
        .assert_called_with("123")
