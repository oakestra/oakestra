import threading

from interfaces import mongodb_requests

instance_ip_lock = threading.Lock()
subnetip_ip_lock = threading.Lock()


def new_job_rr_address(job_data):
    """
    This method is called at deploy time. Given the deployment descriptor check if a custom valid RR ip has been assigned
    by the user and returns that to the service. Otherwise a new RR address will be returned.
    @return: string, a new address
    @raise: exception if an invalid RR address has been provided by the user
    """
    address = job_data.get('RR_ip')
    job_name = job_data['app_name'] + "." + job_data['app_ns'] + "." + job_data['service_name'] + "." + job_data[
        'service_ns']

    if address is not None:
        address_arr = str(address).split(".")
        if len(address_arr) == 4:
            if address_arr[0] != "10" or address_arr[1] != "30":
                raise Exception("RR ip address must be in the form 10.30.x.y")
            job = mongodb_requests.mongo_find_job_by_ip(address)
            if job is not None:
                if job['job_name'] != job_name:
                    raise Exception("RR ip address already used by another service")
            return address
        else:
            raise Exception("Invalid RR_ip address length")
    return new_instance_ip()


def new_instance_ip():
    """
    Function used to assign a new instance IP address for a Service that is going to be deployed.
    An instance address is a static address bounded with a single replica of a service
    @return: string,
        A new address from the address pool. This address is now removed from the pool of available addresses
    """
    with instance_ip_lock:
        addr = mongodb_requests.mongo_get_service_address_from_cache()

        while addr is None:
            addr = mongodb_requests.mongo_get_next_service_ip()
            next_addr = _increase_service_address(addr)
            mongodb_requests.mongo_update_next_service_ip(next_addr)
            job = mongodb_requests.mongo_find_job_by_ip(addr)
            if job is not None:
                addr = None

        return _addr_stringify(addr)


def clear_instance_ip(addr):
    """
    Function used to give back an Instance address to the pool of available addresses
    @param addr: string,
        the address that is going to be added back to the pool
    """
    addr = _addr_destringify(addr)

    # Check if address is in the correct rage
    assert addr[1] == 30
    assert 0 <= addr[2] < 256
    assert 0 <= addr[3] < 256

    with instance_ip_lock:
        next_addr = mongodb_requests.mongo_get_next_service_ip()

        # Ensure that the give address is actually before the next address from the pool
        assert int(str(addr[2]) + str(addr[3])) < int(str(next_addr[2]) + str(next_addr[3]))

        mongodb_requests.mongo_free_service_address_to_cache(addr)


def new_subnetwork_addr():
    """
    Function used to generate a new subnetwork address for any worker node
    @return: string,
        A new address from the address pool. This address is now removed from the pool of available addresses
    """
    with subnetip_ip_lock:
        addr = mongodb_requests.mongo_get_subnet_address_from_cache()

        if addr is None:
            addr = mongodb_requests.mongo_get_next_subnet_ip()
            next_addr = _increase_subnetwork_address(addr)
            mongodb_requests.mongo_update_next_subnet_ip(next_addr)

        return _addr_stringify(addr)


def clear_subnetwork_ip(addr):
    """
    Function used to give back a subnetwork address to the pool of available addresses
    @param addr: string,
        the address that is going to be added back to the pool
    """
    addr = _addr_destringify(addr)

    # Check if address is in the correct rage
    assert 17 < addr[1] < 30
    assert 0 <= addr[2] < 256
    assert addr[3] in [0, 64, 128]

    with subnetip_ip_lock:
        next_addr = mongodb_requests.mongo_get_next_subnet_ip()

        # Ensure that the give address is actually before the next address from the pool
        assert int(str(addr[1]) + str(addr[2]) + str(addr[3])) < int(
            str(next_addr[1]) + str(next_addr[2]) + str(next_addr[3]))

        mongodb_requests.mongo_free_subnet_address_to_cache(addr)


'''
###################### IPv6
'''
# TODO
def new_job_rr_address_v6(job_data):
    """
    This method is called at deploy time. Given the deployment descriptor check if a custom valid RR ip has been assigned
    by the user and returns that to the service. Otherwise a new RR address will be returned.
    @return: string, a new address
    @raise: exception if an invalid RR address has been provided by the user
    """
    address = job_data.get('RR_ip_v6')
    job_name = job_data['app_name'] + "." + job_data['app_ns'] + "." + job_data['service_name'] + "." + job_data[
        'service_ns']

    if address is not None:
        address_arr = str(address).split(".")
        if len(address_arr) == 4:
            if address_arr[0] != "10" or address_arr[1] != "30":
                raise Exception("RR ip address must be in the form 10.30.x.y")
            job = mongodb_requests.mongo_find_job_by_ip(address)
            if job is not None:
                if job['job_name'] != job_name:
                    raise Exception("RR ip address already used by another service")
            return address
        else:
            raise Exception("Invalid RR_ip address length")
    return new_instance_ip()

# TEST
def new_instance_ip_v6():
    """
    Function used to assign a new instance IPv6 address for a Service that is going to be deployed.
    An instance address is a static address bounded with a single replica of a service
    @return: string,
        A new address from the address pool. This address is now removed from the pool of available addresses
    """
    with instance_ip_lock:
        addr = mongodb_requests.mongo_get_service_address_from_cache_v6()

        while addr is None:
            addr = mongodb_requests.mongo_get_next_service_ip_v6()
            next_addr = _increase_service_address_v6(addr)
            mongodb_requests.mongo_update_next_service_ip_v6(next_addr)
            job = mongodb_requests.mongo_find_job_by_ip(addr)
            if job is not None:
                addr = None

        return _addr_stringify_v6(addr)

# TEST
def clear_instance_ip_v6(addr):
    """
    Function used to give back an Instance address to the pool of available addresses
    @param addr: string,
        the address that is going to be added back to the pool
    """
    addr = _addr_destringify_v6(addr)

    # Check if address is in the correct rage
    assert addr[0] == 253
    for n in addr[1:5]:
        assert n == 255

    with instance_ip_lock:
        next_addr = mongodb_requests.mongo_get_next_service_ip_v6()

        # Ensure that the give address is actually before the next address from the pool
        # doing it the ugly way, because for loops are slow
        assert int(str(addr[6]) 
        + str(addr[7])
        + str(addr[8])
        + str(addr[9])
        + str(addr[10])
        + str(addr[11])
        + str(addr[12])
        + str(addr[13])
        + str(addr[14])
        + str(addr[15])
        + str(addr[16])
        ) < int(str(next_addr[6]) 
        + str(next_addr[7])
        + str(next_addr[8])
        + str(next_addr[9])
        + str(next_addr[10])
        + str(next_addr[11])
        + str(next_addr[12])
        + str(next_addr[13])
        + str(next_addr[14])
        + str(next_addr[15])
        + str(next_addr[16])
        )

        mongodb_requests.mongo_free_service_address_to_cache_v6(addr)

# TEST
def new_subnetwork_addr_v6():
    """
    Function used to generate a new subnetwork address for any worker node
    @return: string,
        A new address from the address pool. This address is now removed from the pool of available addresses
    """
    with subnetip_ip_lock:
        addr = mongodb_requests.mongo_get_subnet_address_from_cache_v6()

        if addr is None:
            addr = mongodb_requests.mongo_get_next_subnet_ip_v6()
            next_addr = _increase_subnetwork_address_v6(addr)
            mongodb_requests.mongo_update_next_subnet_ip_v6(next_addr)

        return _addr_stringify_v6(addr)


#TODO
def clear_subnetwork_ip_v6(addr):
    """
    Function used to give back a subnetwork address to the pool of available addresses
    @param addr: string,
        the address that is going to be added back to the pool
    """
    addr = _addr_destringify_v6(addr)

    # Check if address is in the correct rage
    assert 253 < addr[0] < 254
    assert 0 <= addr[2] < 256
    assert addr[3] in [0, 64, 128]

    with subnetip_ip_lock:
        next_addr = mongodb_requests.mongo_get_next_subnet_ip()

        # Ensure that the give address is actually before the next address from the pool
        assert int(str(addr[1]) + str(addr[2]) + str(addr[3])) < int(
            str(next_addr[1]) + str(next_addr[2]) + str(next_addr[3]))

        mongodb_requests.mongo_free_subnet_address_to_cache(addr)

'''
############ Utils
'''

def _increase_service_address(addr):
    new2 = addr[2]
    new3 = (addr[3] + 1) % 254
    if new3 == 0:
        new2 = (addr[2] + 1) % 254
        if new2 == 0:
            raise RuntimeError("Exhausted Address Space")
    return [addr[0], addr[1], new2, new3]


def _increase_service_address_v6(addr):
    # convert subnet portion of addr to int and increase by one
    addr_int = int.from_bytes(addr[6:16], byteorder='big')
    addr_int += 1

    # reconvert new address part to bytearray and concatenate it with the network part of addr
    # will raise OverflowError if address space is exhausted
    new_addr = addr_int.to_bytes(10, byteorder='big')
    new_addr = addr[0:6] + list(new_addr)

    return list(new_addr)


def _increase_subnetwork_address(addr):
    new1 = addr[1]
    new2 = addr[2]
    new3 = addr[3]
    new3 = (new3 + 64) % 256
    if new3 == 0:
        new2 = (new2 + 1) % 256
    if new2 == 0 and new2 != addr[2]:
        new1 = (new1 + 1) % 30
        if new1 == 0:
            raise RuntimeError("Exhausted Address Space")
    return [addr[0], new1, new2, new3]

def _increase_subnetwork_address_v6(addr):
    # convert subnet portion of addr to int and increase by one
    addr_int = int.from_bytes(addr[0:6], byteorder='big')
    addr_int += 1

    # reconvert new subnet part to bytearray and right pad it with 0 to length 16
    new_subnet = addr_int.to_bytes(6, byteorder='big')
    new_subnet += bytes(16 - (len(new_subnet) % 16))

    if new_subnet[0] == 253 and \
       new_subnet[1] == 256 and \
       new_subnet[2] == 256 and \
       new_subnet[3] == 256 and \
       new_subnet[5] == 256 and \
       new_subnet[6] == 256:
       raise RuntimeError("Exhausted IPv6 Address Space")

    return list(new_subnet)


def _addr_stringify(addr):
    res = ""
    for n in addr:
        res = res + str(n) + "."
    return res[0:len(res) - 1]


def _addr_stringify_v6(addr):
    res = ""
    for n in range(0, len(addr), 2):
        res = res + hex(addr[n])[2:4].zfill(2) + hex(addr[n+1])[2:4].zfill(2) + ":"
    return res[0:len(res) - 1]


def _addr_destringify(addrstr):
    addr = []
    for num in addrstr.split("."):
        addr.append(int(num))
    return addr


def _addr_destringify_v6(addrstr):
    addr = []
    for num in addrstr.split(":"):
        addr.append(int(num[0:2], 16))
        addr.append(int(num[2:4], 16))
    return addr