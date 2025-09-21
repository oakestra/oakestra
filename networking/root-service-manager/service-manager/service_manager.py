import os
import socket
from datetime import timedelta

from flask import Flask, request
from flask_jwt_extended import JWTManager
from flask_smorest import Api
from flask_socketio import SocketIO
from flask_swagger_ui import get_swaggerui_blueprint

from blueprints.netinfo_blueprints import netinfoblp
from interfaces.jwt_generator_requests import get_public_key
from interfaces.mongodb_requests import mongo_init
from net_logging import configure_logging
from network import subnetwork_management, routes_interests
from network.utils import sanitize
from operations import instances_management, cluster_management
from operations import service_management

my_logger = configure_logging()

app = Flask(__name__)

# OpenAPI/Swagger Environment
app.config["OPENAPI_VERSION"] = "3.0.2"
app.config["API_TITLE"] = "Oakestra Root Service Manager"
app.config["API_VERSION"] = "v1"
app.config["OPENAPI_URL_PREFIX"] = "/docs"

# JWT Config
app.config["JWT_ALGORITHM"] = "RS256"
app.config["JWT_PUBLIC_KEY"] = get_public_key()
app.config["JWT_ACCESS_TOKEN_EXPIRES"] = timedelta(minutes=10)
app.config["JWT_REFRESH_TOKEN_EXPIRES"] = timedelta(days=7)

app.secret_key = b"\xc8I\xae\x85\x90E\x9aBxQP\xde\x8es\xfdY"
app.logger.addHandler(my_logger)


# OpenAPI/Swagger Configuration
api = Api(app, spec_kwargs={"host": "oakestra.io", "x-internal-id": "1"})
api.DEFAULT_ERROR_RESPONSE_NAME = None
api.spec.components.security_scheme("bearer", {
    "type": "http",
    "scheme": "bearer",
    "bearerFormat": "JWT"
})
api.spec.options["security"] = [{"bearer": []}]
app.register_blueprint(get_swaggerui_blueprint(
    "/api/docs",
    "/docs/openapi.json",
    config={"app_name": "Oakestra Root Service Manager"}
))

api.register_blueprint(netinfoblp)

socketio = SocketIO(
    app,
    async_mode="eventlet",
    logger=True,
    engineio_logger=True,
    cors_allowed_origins="*",
)

jwt = JWTManager(app)

MY_PORT = os.environ.get("MY_PORT") or 10100


# .............. Cluster Registration ..................#
# ......................................................#


@app.route("/api/net/cluster", methods=["POST"])
def register_new_cluster():
    """
    Registration of the new cluster
    json file structure:{
        'cluster_address':str
        'cluster_id':str
        'cluster_port':int
    }
    """
    app.logger.info("Incoming Request /api/net/cluster")
    data = request.json
    app.logger.info(data)

    return cluster_management.register_cluster(
        cluster_id=str(data.get("cluster_id")),
        cluster_port=str(data.get("cluster_port")),
        cluster_address=str(data.get("cluster_address")),
    )


# .............. Cluster Interests .....................#
# ......................................................#


@app.route("/api/net/interest/<job_name>", methods=["DELETE"])
def deregister_cluster_interest(job_name):
    """
    Deregistration of an interest
    json file structure:{
        'job_name':string
    }
    """
    app.logger.info("Incoming Request DELETE /api/net/interest/" + job_name)
    addr = sanitize(request.remote_addr)
    return routes_interests.deregister_interest(addr, job_name)


# ......... Deployment Endpoints .......................#
# ......................................................#


@app.route("/api/net/service/net_deploy_status", methods=["POST"])
def update_instance_local_deployment_addresses():
    """
    Result of the deploy operation in a cluster and the subsequent generated network addresses
    json file structure:{
        'job_id':string
        'instances:[{
            'instance_number':int
            'namespace_ip':string
            'namespace_ip_v6':string
            'host_ip':string
            'host_port':string
        }]
    }
    """

    app.logger.info("Incoming Request /api/net/service/net_deploy_status")
    data = request.json
    app.logger.info(data)

    return instances_management.update_instance_local_addresses(
        instances=data.get("instances"), job_id=data.get("job_id")
    )


@app.route("/api/net/service/deploy", methods=["POST"])
def new_service_deployment():
    """
    Input:
        {
            _id:int,
            deployment_descriptor:{}
        }
    service deployment descriptor and job_id
    The System Manager decorates the service with a new RR Ip in its own DB
    """

    app.logger.info("Incoming Request /api/net/service/deploy")
    data = request.json
    app.logger.info(data)

    return service_management.deploy_request(
        deployment_descriptor=data.get("deployment_descriptor"),
        _id=data.get("_id"),
    )


@app.route("/api/net/service/<_id>", methods=["DELETE"])
def service_undeployment(_id):
    """
    service deployment descriptor and job_id
    The System Manager decorates the service with a new RR Ip in its own DB
    """

    app.logger.info("Incoming Request DELETE /api/net/service/" + _id)

    return service_management.remove_service(_id=str(_id))


@app.route("/api/net/instance/deploy", methods=["POST"])
def new_instance_deployment():
    """
    Input:
        {
            _id:int,
            instance_number:int,
            cluster_id:string,
        }
    The System Manager adds an instance ip for a new deployed instance to a new cluster
    """

    app.logger.info("Incoming Request /api/net/instance/deploy")
    data = request.json
    app.logger.info(data)

    return instances_management.deploy_request(
        sys_job_id=data.get("_id"),
        instance_number=data.get("instance_number"),
        cluster_id=data.get("cluster_id"),
    )


@app.route("/api/net/<_id>/<instance_number>", methods=["DELETE"])
def instance_undeployment(_id, instance_number):
    """
    Undeployment request for the instance number "instance", if instance ==-1 remove the service all together
    """

    app.logger.info(
        "Incoming Request /api/net/undeploy/"
        + str(_id)
        + "/"
        + str(instance_number)
    )

    return instances_management.undeploy_request(
        str(_id), int(instance_number)
    )


# .............. Table query Endpoints .................#
# ......................................................#


@app.route("/api/net/service/<service_name>/instances", methods=["GET"])
def table_query_resolution_by_jobname(service_name):
    """
    Get all the instances of a job given the complete name
    """
    service_name = service_name.replace("_", ".")
    app.logger.info(
        "Incoming Request /api/net/service/" + str(service_name) + "/instances"
    )
    return instances_management.get_service_instances(
        name=service_name, cluster_ip=request.remote_addr
    )


@app.route("/api/net/service/ip/<service_ip>/instances", methods=["GET"])
def table_query_resolution_by_ip(service_ip):
    """
    Get all the instances of a job given a Service IP in 172_30_x_y notation
    """
    service_ip = service_ip.replace("_", ".")
    app.logger.info(
        "Incoming Request /api/net/service/ip/" + str(service_ip) + "/instances"
    )
    return instances_management.get_service_instances(
        ip=service_ip, cluster_ip=request.remote_addr
    )


# ........ Subnetwork management endpoints .............#
# ......................................................#


@app.route("/api/net/subnet", methods=["GET"])
def subnet_request():
    """
    Returns a new subnetwork address
    """
    addr = subnetwork_management.new_subnetwork_addr()
    addrv6 = subnetwork_management.new_subnetwork_addr_v6()
    return {'subnet_addr': addr, 'subnet_addr_v6': addrv6}


if __name__ == "__main__":
    import eventlet

    mongo_init(app)
    eventlet.wsgi.server(
        eventlet.listen(("::", int(MY_PORT)), family=socket.AF_INET6),
        app,
        log=my_logger,
    )
