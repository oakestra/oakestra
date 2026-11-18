from .applications_blueprints import applicationblp, applicationsblp
from .authentication_blueprints import loginbp
from .authorization_blueprints import permissionbp
from .clusters_blueprints import clusterinfo, clustersbp
from .deployment_blueprints import deploybp
from .organization_blueprints import organizationblp
from .scheduling_blueprints import schedulingbp
from .services_blueprints import serviceblp, servicesblp
from .users_blueprints import userbp, usersbp

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
