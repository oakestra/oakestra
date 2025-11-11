from unittest.mock import MagicMock
import sys
from network.subnetwork_management import get_next_available_ip

mongodb_client = sys.modules['interfaces.mongodb_requests']

def test_get_available_ip_from_cache():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=[10, 30, 0, 1])
    mongodb_client.mongo_get_next_service_ip = MagicMock()
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    mongodb_client.mongo_find_job_by_ip = MagicMock()
    
    # test address retrieval from cache
    ip = get_next_available_ip()

    assert ip == "10.30.0.1"
    
    # verify cache was used and DB was not accessed
    mongodb_client.mongo_get_service_address_from_cache.assert_called_once()
    mongodb_client.mongo_get_next_service_ip.assert_not_called()
    # verify no modification of state
    mongodb_client.mongo_update_next_service_ip.assert_not_called()

def test_get_available_ip_with_cache_miss():
    # mock mongo db
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    
    # test address retrieval from DB
    ip = get_next_available_ip()

    assert ip == "10.30.0.1"
    
    # verify DB was accessed but not modified
    mongodb_client.mongo_get_service_address_from_cache.assert_called_once()
    mongodb_client.mongo_get_next_service_ip.assert_called_once()
    mongodb_client.mongo_update_next_service_ip.assert_not_called()

def test_get_available_ip_skip_used_ip():
    # mock mongo db with used IP scenario
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])
    mongodb_client.mongo_find_job_by_ip = MagicMock(side_effect=[{"job_name": "test"}, None])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    
    # test address retrieval skipping used IP
    ip =get_next_available_ip()

    assert ip == "10.30.0.2"
    
    # verify no modification of state
    mongodb_client.mongo_update_next_service_ip.assert_not_called()
    # verify IP usage check
    first_call_args = mongodb_client.mongo_find_job_by_ip.call_args_list[0]
    assert first_call_args[0][0] == [10, 30, 0, 1]

def test_get_available_ip_multiple_calls():
    # mock mongo db with used IPs scenario
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 0, 1])
    mongodb_client.mongo_find_job_by_ip = MagicMock(side_effect=[{"job_name": "test"}, None, {"job_name": "test"}, None])
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    
    # Multiple calls should return the same IP without modifying state
    ip1 = get_next_available_ip()

    assert ip1 == "10.30.0.2"

    ip2 = get_next_available_ip()

    assert ip2 == "10.30.0.2"
    
    # verify no modification of state between calls
    mongodb_client.mongo_update_next_service_ip.assert_not_called()

def test_get_available_ip_exhausted():
    # mock mongo db with exhausted address space
    mongodb_client.mongo_get_service_address_from_cache = MagicMock(return_value=None)
    mongodb_client.mongo_get_next_service_ip = MagicMock(return_value=[10, 30, 253, 253])
    mongodb_client.mongo_find_job_by_ip = MagicMock(return_value=None)
    mongodb_client.mongo_update_next_service_ip = MagicMock()
    
    # test address space exhaustion detection
    ip = get_next_available_ip()

    assert ip is None
    
    # verify no modification even in error case
    mongodb_client.mongo_update_next_service_ip.assert_not_called()