from flask_smorest import Blueprint

from operations import netinfo_management
from utils.security_utils import jwt_auth_required

# ........ Service networking information endpoints ............. #

netinfoblp = Blueprint(
    "netinfo",
    __name__,
    url_prefix="/api/pubnet/service/netinfo",
    description="Network Information API",
)

@netinfoblp.route("/<service_name>", methods=["GET"])
@jwt_auth_required()
def service_netinfo_by_name(service_name):
    """
    Get the networking info of a service given the complete name
    """
    service_name = service_name.replace("_", ".")
    return netinfo_management.get_service_netinfo_by_name(name=service_name)


@netinfoblp.route("/ip/<service_ip>", methods=["GET"])
@jwt_auth_required()
def service_netinfo_by_ip(service_ip):
    """
    Get the networking info of a service given a Service IP in 172_30_x_y notation
    """
    service_ip = service_ip.replace("_", ".")
    return netinfo_management.get_service_netinfo_by_ip(ip=service_ip)
