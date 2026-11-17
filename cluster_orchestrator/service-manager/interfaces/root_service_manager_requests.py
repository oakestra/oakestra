import logging
import os

import requests

logger = logging.getLogger("cluster_service_manager")


CLUSTER_CERT_FILE = os.environ.get("CLUSTER_CERT_FILE")
CLUSTER_KEY_FILE = os.environ.get("CLUSTER_KEY_FILE")
ROOT_CA_FILE = os.environ.get("ROOT_CA_FILE")
ROOT_SERVICE_MANAGER_USE_TLS = os.environ.get("ROOT_SERVICE_MANAGER_USE_TLS", "").lower() in (
    "true",
    "1",
    "yes",
)

# What to verify the root gateway's *server* cert against (same values as cluster_manager):
#   ""         -> the internal root CA
#   "system"   -> the system trust store (root gateway uses a BYO public cert)
#   <path>     -> a custom CA bundle, as a path inside the container
#   "insecure" -> no verification
ROOT_GATEWAY_TRUST = os.environ.get("ROOT_GATEWAY_TRUST", "")


def _mtls_enabled() -> bool:
    if not ROOT_SERVICE_MANAGER_USE_TLS:
        return False
    return all(
        path and os.path.isfile(path)
        for path in (CLUSTER_CERT_FILE, CLUSTER_KEY_FILE, ROOT_CA_FILE)
    )


def _gateway_verify():
    if ROOT_GATEWAY_TRUST == "system":
        return True
    if ROOT_GATEWAY_TRUST == "insecure":
        return False
    return ROOT_GATEWAY_TRUST or ROOT_CA_FILE


_scheme = "https" if _mtls_enabled() else "http"
ROOT_SERVICE_MANAGER_ADDR = (
    _scheme
    + "://"
    + os.environ.get("ROOT_SERVICE_MANAGER_URL", "0.0.0.0")
    + ":"
    + os.environ.get("ROOT_SERVICE_MANAGER_PORT", "5000")
)


def _build_session() -> requests.Session:
    session = requests.Session()
    if _mtls_enabled():
        session.cert = (CLUSTER_CERT_FILE, CLUSTER_KEY_FILE)
        session.verify = _gateway_verify()
    return session


_session = _build_session()


def _cluster_identity_params():
    params = {}
    cluster_address = os.environ.get("CLUSTER_ADDRESS")
    cluster_name = os.environ.get("CLUSTER_NAME")
    if cluster_address:
        params["cluster_address"] = cluster_address
    if cluster_name:
        params["cluster_name"] = cluster_name
    return params


def root_service_manager_get_subnet():
    logger.info("get subnet - logging")
    try:
        response = _session.get(ROOT_SERVICE_MANAGER_ADDR + "/api/net/subnet")
        response.raise_for_status()
        parsed = response.json()
        addr = parsed.get("subnet_addr")
        addrv6 = parsed.get("subnet_addr_v6")
        logger.debug(f"Received subnet: {addr}, subnet_v6: {addrv6}")
        if addr and addrv6 and len(addr) > 0 and len(addrv6) > 0:
            return [addr, addrv6]
        else:
            logger.error(f"Invalid subnet response: addr={addr}, addrv6={addrv6}")
            raise requests.exceptions.RequestException("No address found")
    except requests.exceptions.RequestException as e:
        logger.error(f"Calling System Manager /api/net/subnet not successful: {str(e)}")


def system_manager_notify_deployment_status(job, worker_id):
    logger.info("notify deployment status")
    data = {
        "job_id": str(job["_id"]),
        "instances": [],
    }
    # prepare json data information
    for instance in job["instance_list"]:
        if instance.get("worker_id") == worker_id:
            elem = {
                "instance_number": instance["instance_number"],
                "namespace_ip": instance["namespace_ip"],
                "namespace_ip_v6": instance["namespace_ip_v6"],
                "host_ip": instance["host_ip"],
                "host_port": instance["host_port"],
            }
            data["instances"].append(elem)
    try:
        logger.debug(job)
        _session.post(ROOT_SERVICE_MANAGER_ADDR + "/api/net/service/net_deploy_status", json=data)
    except requests.exceptions.RequestException:
        logger.error("Calling System Manager /api/result/cluster_deploy not successful.")


def root_table_query_ip(ip):
    job_ip = ip.replace(".", "_")
    request_addr = ROOT_SERVICE_MANAGER_ADDR + "/api/net/service/ip/" + str(job_ip) + "/instances"

    params = _cluster_identity_params()

    try:
        return _session.get(request_addr, params=params).json()
    except requests.exceptions.RequestException:
        logger.error("Calling System Manager /api/job/ip/../instances not successful.")


def root_table_query_service_name(name):
    job_name = name.replace(".", "_")
    request_addr = ROOT_SERVICE_MANAGER_ADDR + "/api/net/service/" + str(job_name) + "/instances"

    params = _cluster_identity_params()

    try:
        resp = _session.get(request_addr, params=params)
        return resp.json()
    except requests.exceptions.RequestException as e:
        logger.error(e)
        logger.error("Calling System Manager /api/job/../instances not successful.")


def root_remove_interest(job_name):
    request_addr = ROOT_SERVICE_MANAGER_ADDR + "/api/net/interest/" + str(job_name)

    # Behind the gateway the root sees the proxy as remote_addr, so advertise the address.
    params = _cluster_identity_params() if _mtls_enabled() else None

    try:
        result = _session.delete(request_addr, params=params)
        if result.status_code == 404:
            # TODO(GB): fallback cluster re-register and re-register the interests
            logger.error(result)
            pass
        if result.status_code != 200:
            # TODO(GB): try again later
            logger.error(result)
            pass
    except requests.exceptions.RequestException:
        logger.error("Calling System Manager /api/job/../instances not successful.")
