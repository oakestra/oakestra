from flask_smorest import Blueprint
from marshmallow import fields, Schema

from operations import netinfo_management
from utils.security_utils import jwt_auth_required

# ........ Service networking information endpoints ............. #

netinfoblp = Blueprint(
    "netinfo",
    __name__,
    url_prefix="/api/pubnet/service/netinfo",
    description="Network Information API",
)


class ServiceIpSchema(Schema):
    Address = fields.String(allow_none=True)
    Address_v6 = fields.String(allow_none=True)
    IpType = fields.String(allow_none=True)


class InstanceSchema(Schema):
    cluster_id = fields.String(allow_none=True)
    instance_number = fields.Int(allow_none=True)
    instance_ip = fields.String(allow_none=True)
    instance_ip_v6 = fields.String(allow_none=True)


class ServiceNetinfoSchema(Schema):
    system_job_id = fields.String(allow_none=True)
    applicationID = fields.String(allow_none=True)
    app_ns = fields.String(allow_none=True)
    app_name = fields.String(allow_none=True)
    service_ns = fields.String(allow_none=True)
    service_name = fields.String(allow_none=True)
    service_job_id = fields.String(allow_none=True)
    service_ip_list = fields.Nested(ServiceIpSchema, many=True)
    instance_list = fields.Nested(InstanceSchema, many=True)

class IPQueryArgsSchema(Schema):
    v = fields.String(
        required=False, 
        load_default=None, 
        validate=lambda x: x in ("4", "6")
    )

class AvailableServiceIPsSchema(Schema):
    available_service_ips = fields.Nested(ServiceIpSchema, allow_none=True)

@netinfoblp.route("/available-ip/<x>", methods=["GET"])
@netinfoblp.route("/available-ip/", methods=["GET"])
@netinfoblp.arguments(IPQueryArgsSchema, location="query")
@netinfoblp.response(200, AvailableServiceIPsSchema, content_type="application/json")
def get_available_service_ip(query_args,x=1):
    """
    Get the next x available service IP addresses without reserving them. If x is not asigned, returns a single available IP. 
    Its IP version can be specified via query parameter "v" -> "4" for IPv4, "6" for IPv6, or omit for both.
    """
    version_param = query_args.get("v")
    if version_param == "4":
        version = "v4"
    elif version_param == "6":
        version = "v6"
    else:
        version = None

    return netinfo_management.get_next_x_available_service_ips(int(x),version=version)

@netinfoblp.route("/<service_name>", methods=["GET"])
@netinfoblp.response(200, ServiceNetinfoSchema, content_type="application/json")
@jwt_auth_required()
def service_netinfo_by_name(service_name):
    """
    Get the networking info of a service given the complete name
    """
    service_name = service_name.replace("_", ".")
    return netinfo_management.get_service_netinfo_by_name(name=service_name)


@netinfoblp.route("/ip/<service_ip>", methods=["GET"])
@netinfoblp.response(200, ServiceNetinfoSchema, content_type="application/json")
@jwt_auth_required()
def service_netinfo_by_ip(service_ip):
    """
    Get the networking info of a service given a Service IP in 172_30_x_y notation
    """
    service_ip = service_ip.replace("_", ".")
    return netinfo_management.get_service_netinfo_by_ip(ip=service_ip)
