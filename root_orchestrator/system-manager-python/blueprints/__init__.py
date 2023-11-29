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
