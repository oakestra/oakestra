from unittest.mock import MagicMock
import sys
from network.subnetwork_management import get_next_available_ip, get_next_available_ip_v6

mongodb_client = sys.modules['interfaces.mongodb_requests']

####################### IPv4 TESTS

def test_get_available_ip_from_cache():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache_not_deleting = MagicMock(return_value=[[10, 30, 0, 1]])
    mongodb_client.mongo_get_next_service_ip = MagicMock()
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock()

    # test address retrieval from cache
    ips = get_next_available_ip()

    assert ips == ["10.30.0.1"]

    # verify cache was used and DB was not accessed
    mongodb_client.mongo_get_service_address_from_cache_not_deleting.assert_called_once()
    mongodb_client.mongo_get_next_service_ip.assert_not_called()
    # verify no modification of state
    mongodb_client.mongo_update_next_service_ip.assert_not_called()


def test_get_available_ip_with_cache_miss():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache_not_deleting = MagicMock(return_value=[])
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test address retrieval from DB after cache miss
    ips = get_next_available_ip()

    assert ips == ["10.30.0.1"]

    # verify DB was accessed but not modified
    mongodb_client.mongo_get_service_address_from_cache_not_deleting.assert_called_once()
    mongodb_client.mongo_get_next_service_ip.assert_called_once()


def test_get_available_ip_skip_used_ip():
    # mock mongo db with used IP scenario
    mongodb_client.mongo_get_service_address_from_cache_not_deleting = MagicMock(return_value=[])
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])
    mongodb_client.mongo_find_job_by_ip = MagicMock(side_effect=[{"job_name": "used"}, None])

    # test address retrieval skipping used IP
    ips = get_next_available_ip()

    assert ips == ["10.30.0.2"]

    # verify IP usage check
    first_call_arg = mongodb_client.mongo_find_job_by_ip.call_args_list[0][0][0]
    assert first_call_arg == "10.30.0.1"


def test_get_available_ip_multiple():
    # mock mongo db with used IPs scenario
    mongodb_client.mongo_get_service_address_from_cache_not_deleting = MagicMock(return_value=[])
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])
    mongodb_client.mongo_find_job_by_ip = MagicMock(side_effect=[{"job_name": "used"}, None, None, None])

    # test retrieval of multiple available IPs
    ips = get_next_available_ip(3)

    assert ips == ["10.30.0.2", "10.30.0.3", "10.30.0.4"]




def test_get_available_ip_exhausted():
    # mock mongo db with exhausted address space
    mongodb_client.mongo_get_service_address_from_cache_not_deleting = MagicMock(return_value=[])
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 253, 253])
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value={"job_name": "used"})

    # test address space exhaustion detection
    ips = get_next_available_ip()

    assert ips == []




def test_get_available_ip_cache_and_db_mix():
    # mock mongo db with multiple cached IPs, then fall back to DB
    mongodb_client.mongo_get_service_address_from_cache_not_deleting = MagicMock(
        return_value=[[10, 30, 0, 1], [10, 30, 0, 2]]
    )
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 3])
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test retrieval of multiple IPs from cache and DB
    ips = get_next_available_ip(3)

    assert ips == ["10.30.0.1", "10.30.0.2", "10.30.0.3"]

    # verify both cache and DB were used
    mongodb_client.mongo_get_service_address_from_cache_not_deleting.assert_called_once()
    mongodb_client.mongo_get_next_service_ip.assert_called_once()


def test_get_available_ip_x_greater_than_available():
    # mock mongo db with fewer available IPs than requested
    mongodb_client.mongo_get_service_address_from_cache_not_deleting = MagicMock(
        return_value=[[10, 30, 0, 1], [10, 30, 0, 2]]
    )
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=None)
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test when requested count (x) exceeds available IPs
    ips = get_next_available_ip(5)

    assert ips == ["10.30.0.1", "10.30.0.2"]

    # verify cache was used and DB attempted
    mongodb_client.mongo_get_service_address_from_cache_not_deleting.assert_called_once()
    mongodb_client.mongo_get_next_service_ip.assert_called_once()


####################### IPv6 TESTS


def test_get_available_ip_v6_from_cache():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6 = MagicMock(
        return_value=[[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]]
    )
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock()

    # test address retrieval from cache
    ips = get_next_available_ip_v6()

    assert len(ips) == 1
    assert ips[0].startswith("fdff")

    # verify cache was used and DB was not accessed
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6.assert_called_once()
    mongodb_client.mongo_get_next_service_ip_v6.assert_not_called()


def test_get_available_ip_v6_with_cache_miss():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6 = MagicMock(return_value=[])
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(
        return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]
    )
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test retrieval from DB after cache miss
    ips = get_next_available_ip_v6()

    assert len(ips) == 1
    assert ips[0].startswith("fdff")

    # verify DB was accessed but not modified
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6.assert_called_once()
    mongodb_client.mongo_get_next_service_ip_v6.assert_called_once()


def test_get_available_ip_v6_skip_used_ip():
    # mock mongo db with used IPv6 scenario
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6 = MagicMock(return_value=[])
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(
        return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]
    )
    mongodb_client.mongo_find_job_by_ip = MagicMock(side_effect=[{"job_name": "used"}, None])

    # test skipping a used IPv6 address
    ips = get_next_available_ip_v6()

    assert len(ips) == 1
    assert ips[0].startswith("fdff")

    # verify IP usage check
    first_call_arg = mongodb_client.mongo_find_job_by_ip.call_args_list[0][0][0]
    assert isinstance(first_call_arg, str) and first_call_arg.startswith("fdff")


def test_get_available_ip_v6_multiple():
    # mock mongo db with multiple IPv6 addresses available
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6 = MagicMock(return_value=[])
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(
        return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]
    )
    mongodb_client.mongo_find_job_by_ip = MagicMock(side_effect=[{"job_name": "used"}, None, None, None])

    # test retrieval of multiple IPv6 addresses
    ips = get_next_available_ip_v6(3)

    assert len(ips) == 3
    assert all(ip.startswith("fdff") for ip in ips)



def test_get_available_ip_v6_exhausted():
    # mock mongo db with exhausted IPv6 address space
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6 = MagicMock(return_value=[])
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test exhaustion case
    ips = get_next_available_ip_v6()

    assert ips == []




def test_get_available_ip_v6_cache_and_db_mix():
    # mock mongo db with multiple cached IPv6 addresses, then fall back to DB
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6 = MagicMock(
        return_value=[
            [253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1],
            [253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2],
            
        ]
    )
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(
        return_value=[253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3]
    )
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test retrieval of multiple IPv6 addresses from cache and DB
    ips = get_next_available_ip_v6(3)

    assert len(ips) == 3
    assert all(ip.startswith("fdff") for ip in ips)

    # verify both cache and DB were used
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6.assert_called_once()
    mongodb_client.mongo_get_next_service_ip_v6.assert_called_once()



def test_get_available_ip_v6_x_greater_than_available():
    # mock mongo db with fewer available IPv6 addresses than requested
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6 = MagicMock(
        return_value=[
            [253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1],
            [253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2]
        ]
    )
    mongodb_client.mongo_get_next_service_ip_v6 = MagicMock(return_value=None)
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)

    # test when requested count (x) exceeds available IPv6 addresses
    ips = get_next_available_ip_v6(5)

    assert len(ips) == 2
    assert all(ip.startswith("fdff") for ip in ips)

    # verify cache was used and DB attempted
    mongodb_client.mongo_get_service_address_from_cache_not_deleting_v6.assert_called_once()
    mongodb_client.mongo_get_next_service_ip_v6.assert_called_once()
