from unittest.mock import MagicMock
import sys
from network.subnetwork_management import *

mongodb_client = sys.modules['interfaces.mongodb_requests']

def test_instance_address_base():
    mongodb_client.mongo_get_service_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip_v6= MagicMock(return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)
    mongodb_client.mongo_update_next_service_ip_v6 = MagicMock()

    ip1 = new_instance_ip_v6()
    assert ip1 == "fdff::"

    mongodb_client.mongo_update_next_service_ip_v6.assert_called_with([253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1])


def test_instance_address_complex():
    mongodb_client.mongo_get_service_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 1, 1])
    mongodb_client.mongo_update_next_service_ip_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    ip1 = new_instance_ip_v6()
    assert ip1 == "fdff::2:0:101"

    mongodb_client.mongo_update_next_service_ip_v6.assert_called_with([253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 1, 2])


def test_instance_address_recycle():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_update_next_service_ip_v6 = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_instance_ip_v6()
    assert ip1 == "fdff::"

    # mock next address
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1])

    # test clearance condition
    clear_instance_ip_v6(ip1)

    mongodb_client.mongo_free_service_address_to_cache_v6.assert_called_with([253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])

def test_instance_address_recycle_failure_1():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_update_next_service_ip_v6 = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_instance_ip_v6()
    assert ip1 == "fdff::"

    # mock next address
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1])

    # test clearance condition
    passed = False
    try:
        clear_instance_ip_v6("fdff::1")
        passed = True
    except:
        pass
    assert passed is False


def test_instance_address_recycle_failure_2():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_update_next_service_ip_v6 = MagicMock()
    mongodb_client.mongo_free_service_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_instance_ip_v6()
    assert ip1 == "fdff::"

    # mock next address
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1])

    # test clearance condition
    passed = False
    try:
        clear_instance_ip_v6("fdfe::")
        passed = True
    except:
        pass
    assert passed is False


def test_subnet_address_base():
    mongodb_client.mongo_get_subnet_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_update_next_subnet_ip_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    ip1 = new_subnetwork_addr_v6()
    assert ip1 == "fc00::"

    mongodb_client.mongo_update_next_subnet_ip_v6.assert_called_with([252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0])


def test_subnet_address_complex_1():
    mongodb_client.mongo_get_subnet_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 0, 0, 0, 0, 127, 255, 255, 255, 255, 255, 255, 0])
    mongodb_client.mongo_update_next_subnet_ip_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    ip1 = new_subnetwork_addr_v6()
    assert ip1 == "fc00::7fff:ffff:ffff:ff00"

    mongodb_client.mongo_update_next_subnet_ip_v6.assert_called_with([252, 0, 0, 0, 0, 0, 0, 0, 128, 0, 0, 0, 0, 0, 0, 0])


def test_subnet_address_complex_2():
    mongodb_client.mongo_get_subnet_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 7, 0, 0, 1, 255, 5, 4, 3, 2, 255, 255, 0])
    mongodb_client.mongo_update_next_subnet_ip_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    ip1 = new_subnetwork_addr_v6()
    assert ip1 == "fc00:0:700:1:ff05:403:2ff:ff00"

    mongodb_client.mongo_update_next_subnet_ip_v6.assert_called_with([252, 0, 0, 0, 7, 0, 0, 1, 255, 5, 4, 3, 3, 0, 0, 0])


def test_subnet_address_exhausted():
    mongodb_client.mongo_get_subnet_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[253, 252, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 0])
    mongodb_client.mongo_update_next_subnet_ip_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    try:
        ip1 = new_subnetwork_addr_v6()
        assert False
    except:
        assert True


def test_subnet_address_recycle():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0])
    mongodb_client.mongo_update_next_subnet_ip_v6 = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_subnetwork_addr_v6()
    assert ip1 == "fc00::100"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0])

    # test clearance condition
    clear_subnetwork_ip_v6(ip1)

    mongodb_client.mongo_free_subnet_address_to_cache_v6.assert_called_with([252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0])


def test_subnet_address_recycle_failure_1():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0])
    mongodb_client.mongo_update_next_subnet_ip_v6 = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_subnetwork_addr_v6()
    assert ip1 == "fc00::100"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0])

    passed = False
    try:
        clear_subnetwork_ip_v6("fc00::200")
        passed = True
    except:
        pass
    assert passed is False


def test_subnet_address_recycle_failure_2():
    # mock mongo db
    mongodb_client.mongo_get_subnet_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0])
    mongodb_client.mongo_update_next_subnet_ip_v6 = MagicMock()
    mongodb_client.mongo_free_subnet_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address generation
    ip1 = new_subnetwork_addr_v6()
    assert ip1 == "fc00::100"

    # mock next address
    mongodb_client.mongo_get_next_subnet_ip_v6 = MagicMock(return_value=[252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0])

    passed = False
    try:
        clear_subnetwork_ip_v6("fc00:0:dead:beef:c01d:c0ff:ee00:0")
        passed = True
    except:
        pass
    assert passed is False


def test_new_job_rr_address():
    mongodb_client.mongo_get_rr_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_rr_ip_v6 = MagicMock(return_value=[253, 255, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_update_next_rr_ip_v6 = MagicMock()
    mongodb_client.mongo_free_rr_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    file = {
        'RR_ip': '10.30.0.1',
        'RR_ip_v6': 'fdff:2000::1337',
        'app_name': 's1',
        'app_ns': 'test',
        'service_name': 's1',
        'service_ns': 'test'
    }

    addr = new_job_rr_address_v6(file)

    assert 'fdff:2000::1337' == addr


def test_new_job_rr_address_fail1():
    mongodb_client.mongo_get_rr_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_rr_ip_v6 = MagicMock(return_value=[253, 255, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_update_next_rr_ip_v6 = MagicMock()
    mongodb_client.mongo_free_rr_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    file = {
        'RR_ip': '10.20.0.1',
        'RR_ip_v6': 'fdff:1000::1337',
        'app_name': 's1',
        'app_ns': 'test',
        'service_name': 's1',
        'service_ns': 'test'
    }

    passed = False
    try:
        addr = new_job_rr_address_v6(file)
        passed = True
    except:
        pass

    assert passed is False


def test_new_job_rr_address_fail2():
    mongodb_client.mongo_get_rr_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_rr_ip_v6 = MagicMock(return_value=[253, 255, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_update_next_rr_ip_v6 = MagicMock()
    mongodb_client.mongo_free_rr_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    file = {
        'RR_ip': '173.30.0.1',
        'RR_ip_v6': '2001:db80::1337',
        'app_name': 's1',
        'app_ns': 'test',
        'service_name': 's1',
        'service_ns': 'test'
    }

    passed = False
    try:
        addr = new_job_rr_address_v6(file)
        passed = True
    except:
        pass

    assert passed is False


def test_new_job_rr_address_fail3():
    mongodb_client.mongo_get_rr_address_from_cache_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_rr_ip_v6 = MagicMock(return_value=[253, 255, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    mongodb_client.mongo_update_next_rr_ip_v6 = MagicMock()
    mongodb_client.mongo_free_rr_address_to_cache_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    file = {
        'app_name': 's1',
        'app_ns': 'test',
        'service_name': 's1',
        'service_ns': 'test'
    }

    addr = new_job_rr_address_v6(file)
    assert addr == 'fdff:2000::'
