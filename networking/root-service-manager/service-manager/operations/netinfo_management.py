from network import tablequery


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
