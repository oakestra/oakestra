from unittest.mock import MagicMock
from operations import cluster_management
import sys

mongodb_client = sys.modules['interfaces.mongodb_requests']


def _get_fake_cluster():
    return {
        "cluster_id": "1",
        "cluster_info": {
            "cluster_address": "192.168.1.1",
            "cluster_port": "5555",
            "status": cluster_management.CLUSTER_STATUS_ACTIVE
        },
        "instances": ["aaa", "bbb"],
    }


def test_register_cluster():
    fake_cluster = _get_fake_cluster()
    mongodb_client.mongo_cluster_add = MagicMock()

    result, code = cluster_management.register_cluster(
        fake_cluster["cluster_id"],
        fake_cluster["cluster_info"]["cluster_port"],
        fake_cluster["cluster_info"]["cluster_address"]
    )

    assert code == 200

    mongodb_client. \
        mongo_cluster_add. \
        assert_called_with(
        fake_cluster["cluster_id"],
        fake_cluster["cluster_info"]
    )
