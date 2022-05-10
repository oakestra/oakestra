from blueprints.scheduling_blueprints import schedulingbp
from blueprints.services_blueprints import serviceblp, servicesblp
from blueprints.authorization_blueprints import permissionbp
from blueprints.authentication_blueprints import loginbp
from blueprints.deployment_blueprints import deploybp
from blueprints.applications_blueprints import applicationsblp,applicationblp
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
    schedulingbp
]
