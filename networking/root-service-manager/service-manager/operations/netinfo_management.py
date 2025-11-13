from network import tablequery
from network.subnetwork_management import get_next_available_ip,get_next_available_ip_v6

def get_service_netinfo_by_ip(ip):
    return _get_service_info_internal(tablequery.service_resolution(ip=ip))


def get_service_netinfo_by_name(name):
    return _get_service_info_internal(tablequery.service_resolution(name=name))


def _get_service_info_internal(job):
    if job is None:
        return "Service not found", 404

    return _convert_job_to_netinfo(job), 200


def _convert_job_to_netinfo(job):
    netinfo = _subdict(job, [
        "system_job_id",
        "applicationID",
        "app_ns",
        "app_name",
        "service_ns",
        "service_name"
    ])

    service_job_id = job.get("_id")
    if service_job_id is not None:
        netinfo["service_job_id"] = str(service_job_id)

    service_ip_list = []
    for service_ip in job.get("service_ip_list", []):
        service_ip_list.append(_subdict(service_ip, [
            "Address",
            "Address_v6",
            "IpType"
        ]))
    netinfo["service_ip_list"] = service_ip_list

    instance_list = []
    for instance in job.get("instance_list", []):
        instance_list.append(_subdict(instance, [
            "cluster_id",
            "instance_number",
            "instance_ip",
            "instance_ip_v6",
        ]))
    netinfo["instance_list"] = instance_list

    return netinfo


def _subdict(src_dict, keys):
    dst_dict = {}
    for key, value in src_dict.items():
        if key in keys and value is not None:
            dst_dict[key] = value

    return dst_dict

def _format_service_ip(addr_v4=None, addr_v6=None):
    if addr_v4:
        return {
            "Address": addr_v4,
            "Address_v6": None,
            "IpType": "IPv4"
        }
    elif addr_v6:
        return {
            "Address": None,
            "Address_v6": addr_v6,
            "IpType": "IPv6"
        }
    else:
        return None

def get_next_x_available_service_ips(x=1, version=None):
    """
    Return next x available IPv4 and/or IPv6 addresses formatted per schema.
    Does not modify the address pool (read-only).
    """
    ips = {"available_service_ips": []}

    ipv4_list = []
    ipv6_list = []

    if version is None:
        ipv4_list = get_next_available_ip(x)
        ipv6_list = get_next_available_ip_v6(x)
    elif version == "v4":
        ipv4_list = get_next_available_ip(x)
    elif version == "v6":
        ipv6_list = get_next_available_ip_v6(x)


    for ip in ipv4_list:
        ips["available_service_ips"].append(_format_service_ip(addr_v4=ip))
        
    for ip in ipv6_list:
        ips["available_service_ips"].append(_format_service_ip(addr_v6=ip))

    return ips
