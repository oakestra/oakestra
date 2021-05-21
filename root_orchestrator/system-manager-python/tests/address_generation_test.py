from unittest.mock import MagicMock
import unittest
import sys
sys.modules['mongodb_client'] = unittest.mock.Mock()
import service_manager

mongodb_client = sys.modules['mongodb_client']

def test_instance_address_base():
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()

    ip1 = service_manager.new_instance_ip()
    assert ip1 == "172.30.0.0"

    mongodb_client.mongo_update_next_service_ip.assert_called_with([172, 30, 0, 1])

def test_instance_address_complex():
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 253])
    mongodb_client.mongo_update_next_service_ip = MagicMock()

    ip1 = service_manager.new_instance_ip()
    assert ip1 == "172.30.0.253"

    mongodb_client.mongo_update_next_service_ip.assert_called_with([172, 30, 1, 0])


def test_instance_address_address_recycle():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_address_to_cache = MagicMock()

    # test address generation
    ip1 = service_manager.new_instance_ip()
    assert ip1 == "172.30.0.0"

    # mock next address
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 1])

    # test clearance condition
    service_manager.clear_instance_ip(ip1)

    mongodb_client.mongo_free_address_to_cache.assert_called_with([172, 30, 0, 0])

def test_instance_address_address_recycle_failure():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_address_to_cache = MagicMock()

    # test address generation
    ip1 = service_manager.new_instance_ip()
    assert ip1 == "172.30.0.0"

    # test clearance condition
    try:
        service_manager.clear_instance_ip(ip1)
        raise RuntimeError("Should have thrown an exception")
    except:
        assert True
