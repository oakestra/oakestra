from unittest.mock import MagicMock
import sys
from network.subnetwork_management import *

mongodb_client = sys.modules['interfaces.mongodb_requests']


def test_instance_address_base():
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 0])
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)
    mongodb_client.mongo_update_next_service_ip = MagicMock()

    ip1 = new_instance_ip()
    assert ip1 == "10.30.0.0"

    mongodb_client.mongo_update_next_service_ip.assert_called_with([10, 30, 0, 1])


def test_instance_address_complex():
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 253])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    ip1 = new_instance_ip()
    assert ip1 == "10.30.0.253"

    mongodb_client.mongo_update_next_service_ip.assert_called_with([10, 30, 1, 0])


def test_instance_address_recycle():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_instance_ip()
    assert ip1 == "10.30.0.0"

    # mock next address
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])

    # test clearance condition
    clear_instance_ip(ip1)

    mongodb_client.mongo_free_service_address_to_cache.assert_called_with([10, 30, 0, 0])


def test_instance_address_recycle_failure_1():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_instance_ip()
    assert ip1 == "10.30.0.0"

    # mock next address
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])

    # test clearance condition
    passed = False
    try:
        clear_instance_ip("10.30.0.1")
        passed = True
    except:
        pass
    assert passed is False


def test_instance_address_recycle_failure_2():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_instance_ip()
    assert ip1 == "10.30.0.0"

    # mock next address
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])

    # test clearance condition
    passed = False
    try:
        clear_instance_ip("10.29.0.0")
        passed = True
    except:
        pass
    assert passed is False


def test_subnet_address_base():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 18, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    ip1 = new_subnetwork_addr()
    assert ip1 == "10.18.0.0"

    mongodb_client.mongo_update_next_subnet_ip.assert_called_with([10, 18, 0, 64])


def test_subnet_address_complex_1():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 18, 255, 192])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    ip1 = new_subnetwork_addr()
    assert ip1 == "10.18.255.192"

    mongodb_client.mongo_update_next_subnet_ip.assert_called_with([10, 19, 0, 0])


def test_subnet_address_complex_2():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 18, 254, 128])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    ip1 = new_subnetwork_addr()
    assert ip1 == "10.18.254.128"

    mongodb_client.mongo_update_next_subnet_ip.assert_called_with([10, 18, 254, 192])


def test_subnet_address_exhausted():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 29, 255, 192])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    try:
        ip1 = new_subnetwork_addr()
        assert False
    except:
        assert True


def test_subnet_address_recycle():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_subnetwork_addr()
    assert ip1 == "10.20.0.0"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 64])

    # test clearance condition
    clear_subnetwork_ip(ip1)

    mongodb_client.mongo_free_subnet_address_to_cache.assert_called_with([10, 20, 0, 0])


def test_subnet_address_recycle_failure_1():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_subnetwork_addr()
    assert ip1 == "10.20.0.0"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 64])

    passed = False
    try:
        clear_subnetwork_ip("10.20.0.64")
        passed = True
    except:
        pass
    assert passed is False


def test_subnet_address_recycle_failure_2():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_subnetwork_addr()
    assert ip1 == "10.20.0.0"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 64])

    passed = False
    try:
        clear_subnetwork_ip("10.12.0.0")
        passed = True
    except:
        pass
    assert passed is False


def test_new_job_rr_address():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    file = {
        'RR_ip': '10.30.0.1',
        'app_name': 's1',
        'app_ns': 'test',
        'service_name': 's1',
        'service_ns': 'test'
    }

    addr = new_job_rr_address(file)

    assert '10.30.0.1' == addr


def test_new_job_rr_address_fail1():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    file = {
        'RR_ip': '10.20.0.1',
        'app_name': 's1',
        'app_ns': 'test',
        'service_name': 's1',
        'service_ns': 'test'
    }

    passed = False
    try:
        addr = new_job_rr_address(file)
        passed = True
    except:
        pass

    assert passed is False


def test_new_job_rr_address_fail2():
    mongodb_client.mongo_get_subnet_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip = MagicMock(return_value=[10, 20, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    file = {
        'RR_ip': '173.30.0.1',
        'app_name': 's1',
        'app_ns': 'test',
        'service_name': 's1',
        'service_ns': 'test'
    }

    passed = False
    try:
        addr = new_job_rr_address(file)
        passed = True
    except:
        pass

    assert passed is False


def test_new_job_rr_address_fail3():
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 0])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    file = {
        'app_name': 's1',
        'app_ns': 'test',
        'service_name': 's1',
        'service_ns': 'test'
    }

    addr = new_job_rr_address(file)

    assert addr == '10.30.0.0'

