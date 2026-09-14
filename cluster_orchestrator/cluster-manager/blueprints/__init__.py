import config

from blueprints.cluster_blueprints import clusterblp
from blueprints.service_blueprints import schedulingblp, serviceblp
from blueprints.worker_blueprints import workerblp

blueprints = [serviceblp, schedulingblp, workerblp, clusterblp]

if config.GATEWAY_ENABLED:
    # Certificate APIs only exist behind the gateway.
    from blueprints.certificates_blueprints import certbp

    blueprints.append(certbp)
