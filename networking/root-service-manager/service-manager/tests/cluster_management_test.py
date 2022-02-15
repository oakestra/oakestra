from unittest.mock import MagicMock
from operations import cluster_management
from network import routes_interests
import sys

mongodb_client = sys.modules['interfaces.mongodb_requests']


def _get_fake_cluster():
    return {
        "_id": "1",
        "cluster_id": "1",
        "cluster_address": "192.168.1.1",
        "cluster_port": "5555",
        "status": cluster_management.CLUSTER_STATUS_ACTIVE,
        "instances": ["aaa", "bbb"],
    }


def test_register_cluster():
    fake_cluster = _get_fake_cluster()
    mongodb_client.mongo_cluster_add = MagicMock()

    result, code = cluster_management.register_cluster(
        cluster_id=fake_cluster["cluster_id"],
        cluster_port=fake_cluster["cluster_port"],
        cluster_address=fake_cluster["cluster_address"]
    )

    assert code == 200

    mongodb_client. \
        mongo_cluster_add. \
        assert_called_with(
        cluster_id=fake_cluster["cluster_id"],
        cluster_port=fake_cluster["cluster_port"],
        cluster_address=fake_cluster["cluster_address"],
        status=fake_cluster["status"]
    )


def test_deregister_cluster_interest():
    fake_cluster = _get_fake_cluster()
    mongodb_client.mongo_get_cluster_by_ip = MagicMock(return_value=fake_cluster)
    mongodb_client.mongo_remove_cluster_job_interest = MagicMock()

    result, code = routes_interests.deregister_interest(
        fake_cluster["cluster_address"],
        "aaa"
    )

    assert code == 200

    mongodb_client. \
        mongo_remove_cluster_job_interest. \
        assert_called_with(
        fake_cluster["cluster_id"],
        "aaa"
    )
