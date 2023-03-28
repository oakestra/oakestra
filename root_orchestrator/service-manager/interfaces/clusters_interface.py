import logging
import requests
from requests.adapters import HTTPAdapter
from requests.packages.urllib3.util.retry import Retry

NOTIFY_INTEREST_ENDPOINT = "/api/net/job/update"

logging.basicConfig(level=logging.DEBUG)


def notify_undeployment(cluster_addr, cluster_port, job_name, instancenum):
    logging.debug("Notifying undeployment of " + job_name + " to a cluster")
    return _notify_interest_update(cluster_addr, cluster_port, job_name, instancenum, "UNDEPLOYMENT")


def notify_deployment(cluster_addr, cluster_port, job_name, instancenum):
    logging.debug("Notifying deployment of " + job_name + " to a cluster")
    return _notify_interest_update(cluster_addr, cluster_port, job_name, instancenum, "DEPLOYMENT")


def _notify_interest_update(cluster_addr, cluster_port, job_name, instancenum, type):
    return request_with_retry(
        url="http://[" + str(cluster_addr) + "]:" + str(cluster_port) + NOTIFY_INTEREST_ENDPOINT,
        json={
            "job_name": job_name,
            "instance_number": instancenum,
            "type": type
        })


def request_with_retry(url, json):
    s = requests.Session()
    retries = Retry(total=5, backoff_factor=1, status_forcelist=[502, 503, 504])
    s.mount('http://', HTTPAdapter(max_retries=retries))

    session = s.post(url=url, json=json, timeout=2)
    return session.status_code
