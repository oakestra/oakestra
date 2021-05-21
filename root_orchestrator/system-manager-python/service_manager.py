import mongodb_client


def new_instance_ip():
    """
    Function used to assign a new instance IP address for a Service that is going to be deployed.
    An instance address is a static address bounded with a single replica of a service
    @return: string,
        A new address from the address pool. This address is now removed from the pool of available addresses
    """
    addr = mongodb_client.mongo_get_service_address_from_cache()

    if addr is None:
        addr = mongodb_client.mongo_get_next_service_ip()
        next_addr = _increase_address(addr)
        mongodb_client.mongo_update_next_service_ip(next_addr)

    return _addr_stringify(addr)


def clear_instance_ip(addr):
    """
    Function used to give back an Instance address to the pool of available addresses
    @param addr: string,
        the address that is going to be added back to the pool
    """
    addr = _addr_destringify(addr)

    next_addr = mongodb_client.mongo_get_next_service_ip()

    # Ensure that the give address is actually before the next address from the pool
    assert int(str(addr[2]) + str(addr[3])) < int(str(next_addr[2]) + str(next_addr[3]))

    mongodb_client.mongo_free_address_to_cache(addr)


def service_resolution():
    pass


def _increase_address(addr):
    new2 = addr[2]
    new3 = (addr[3] + 1) % 254
    if new3 is 0:
        new2 = (addr[2] + 1) % 254
        if new2 is 0:
            raise RuntimeError("Exhausted Address Space")
    return [addr[0], addr[1], new2, new3]


def _addr_stringify(addr):
    res = ""
    for n in addr:
        res = res + str(n) + "."
    return res[0:len(res) - 1]


def _addr_destringify(addrstr):
    addr = []
    for num in addrstr.split("."):
        addr.append(int(num))
    return addr
