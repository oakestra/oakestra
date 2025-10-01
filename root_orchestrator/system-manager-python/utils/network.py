import socket
from urllib.parse import unquote


def is_ipv6(address):
    """Checks if the given address is a valid IPv6 address."""
    try:
        socket.inet_pton(socket.AF_INET6, address)
        return True
    except socket.error:
        return False


def add_brackets_if_ipv6(address):
    """Adds brackets to the address if it's IPv6 and doesn't have them."""
    if is_ipv6(address) and not address.startswith("["):
        return f"[{address}]"
    else:
        return address


def is_4to6_mapped(address):
    """Checks if the given address is 4-to-6 mapped."""
    return is_ipv6(address) and address.startswith("::")


def extract_v4_address_from_v6_mapped(address):
    """Returns IPv4 address, given address is a 4-to-6 mapped IP address"""
    return address.split(":")[3]


def sanitize(address, request=False):
    if is_4to6_mapped(address):
        return extract_v4_address_from_v6_mapped(address)
    if request:
        return add_brackets_if_ipv6(address)
    return address


def get_ip_from_grpc_transport(transport):
    """Extracts the IP address from the gRPC transport string."""
    transport_parts = transport.split(":")  # grpc format is protocol:ip:port
    l3_protocol = transport_parts[0]
    transport_port = transport_parts[len(transport_parts) - 1]
    url = unquote(transport)
    cluster_ip = ""

    if l3_protocol == "ipv4":
        cluster_ip = transport_parts[1]
    elif l3_protocol == "ipv6":
        cluster_ip = url.replace("ipv6:", "")
        cluster_ip = cluster_ip.replace(":" + transport_port, "")

    return sanitize(cluster_ip)
