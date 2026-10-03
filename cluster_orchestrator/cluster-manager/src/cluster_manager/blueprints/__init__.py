from .cluster_blueprints import clusterblp
from .service_blueprints import schedulingblp, serviceblp
from .worker_blueprints import workerblp

blueprints = [serviceblp, schedulingblp, workerblp, clusterblp]
