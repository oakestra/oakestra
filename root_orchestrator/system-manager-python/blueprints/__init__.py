from blueprints.services_blueprints import serviceblp, servicesblp
from blueprints.authorization_blueprints import permissionbp
from blueprints.authentication_blueprints import loginbp
from blueprints.deployment_blueprints import deploybp

blueprints = [
    serviceblp,
    servicesblp,
    permissionbp,
    loginbp,
    deploybp
]
