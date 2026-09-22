from utils.gateway import GATEWAY_ENABLED

from blueprints.applications_blueprints import applicationblp, applicationsblp
from blueprints.authentication_blueprints import loginbp
from blueprints.authorization_blueprints import permissionbp
from blueprints.clusters_blueprints import clusterinfo, clustersbp
from blueprints.deployment_blueprints import deploybp
from blueprints.organization_blueprints import organizationblp
from blueprints.scheduling_blueprints import schedulingbp
from blueprints.services_blueprints import serviceblp, servicesblp
from blueprints.users_blueprints import userbp, usersbp

blueprints = [
    serviceblp,
    servicesblp,
    permissionbp,
    loginbp,
    deploybp,
    applicationblp,
    applicationsblp,
    userbp,
    usersbp,
    schedulingbp,
    clusterinfo,
    clustersbp,
    organizationblp,
]

if GATEWAY_ENABLED:
    # Certificate and registration-token APIs only exist behind the gateway.
    from blueprints.certificates_blueprints import certbp
    from blueprints.registration_tokens_blueprints import tokensbp

    blueprints += [certbp, tokensbp]
