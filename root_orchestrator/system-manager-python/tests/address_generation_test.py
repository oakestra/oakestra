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


def test_instance_address_recycle():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache = MagicMock()

    # test address generation
    ip1 = service_manager.new_instance_ip()
    assert ip1 == "172.30.0.0"

    # mock next address
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 1])

    # test clearance condition
    service_manager.clear_instance_ip(ip1)

    mongodb_client.mongo_free_service_address_to_cache.assert_called_with([172, 30, 0, 0])


def test_instance_address_recycle_failure_1():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache = MagicMock()

    # test address generation
    ip1 = service_manager.new_instance_ip()
    assert ip1 == "172.30.0.0"

    # mock next address
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 1])

    # test clearance condition
    passed = False
    try:
        service_manager.clear_instance_ip("172.30.0.1")
        passed = True
    except:
        pass
    assert passed is False


def test_instance_address_recycle_failure_2():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache = MagicMock()

    # test address generation
    ip1 = service_manager.new_instance_ip()
    assert ip1 == "172.30.0.0"

    # mock next address
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[172, 30, 0, 1])

    # test clearance condition
    passed = False
    try:
        service_manager.clear_instance_ip("172.29.0.0")
        passed = True
    except:
        pass
    assert passed is False


def test_subnet_address_base():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 18, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()

    ip1 = service_manager.new_subnetwork_addr()
    assert ip1 == "172.18.0.0"

    mongodb_client.mongo_update_next_subnet_ip.assert_called_with([172, 18, 0, 64])


def test_subnet_address_complex_1():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 18, 255, 192])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()

    ip1 = service_manager.new_subnetwork_addr()
    assert ip1 == "172.18.255.192"

    mongodb_client.mongo_update_next_subnet_ip.assert_called_with([172, 19, 0, 0])


def test_subnet_address_complex_2():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 18, 254, 128])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()

    ip1 = service_manager.new_subnetwork_addr()
    assert ip1 == "172.18.254.128"

    mongodb_client.mongo_update_next_subnet_ip.assert_called_with([172, 18, 254, 192])


def test_subnet_address_exhausted():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 29, 255, 192])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()

    try:
        ip1 = service_manager.new_subnetwork_addr()
        assert False
    except:
        assert True


def test_subnet_address_recycle():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()

    # test address generation
    ip1 = service_manager.new_subnetwork_addr()
    assert ip1 == "172.20.0.0"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 20, 0, 64])

    # test clearance condition
    service_manager.clear_subnetwork_ip(ip1)

    mongodb_client.mongo_free_subnet_address_to_cache.assert_called_with([172, 20, 0, 0])


def test_subnet_address_recycle_failure_1():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()

    # test address generation
    ip1 = service_manager.new_subnetwork_addr()
    assert ip1 == "172.20.0.0"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 20, 0, 64])

    passed = False
    try:
        service_manager.clear_subnetwork_ip("172.20.0.64")
        passed = True
    except:
        pass
    assert passed is False


def test_subnet_address_recycle_failure_2():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()

    # test address generation
    ip1 = service_manager.new_subnetwork_addr()
    assert ip1 == "172.20.0.0"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[172, 20, 0, 64])

    passed = False
    try:
        service_manager.clear_subnetwork_ip("172.12.0.0")
        passed = True
    except:
        pass
    assert passed is False
