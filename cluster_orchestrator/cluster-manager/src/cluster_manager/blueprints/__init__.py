from .. import gateway_config
from .cluster_blueprints import clusterblp
from .service_blueprints import schedulingblp, serviceblp
from .worker_blueprints import workerblp

blueprints = [serviceblp, schedulingblp, workerblp, clusterblp]

if gateway_config.GATEWAY_ENABLED:
    # Certificate APIs only exist behind the gateway.
    from .certificates_blueprints import certbp

    blueprints.append(certbp)
