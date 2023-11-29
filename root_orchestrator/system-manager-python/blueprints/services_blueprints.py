import logging

import sla.schema
from blueprints.schema_wrapper import SchemaWrapper
from bson import json_util
from flask import request
from flask.views import MethodView
from flask_restful import Resource
from flask_smorest import Blueprint, abort
from roles.securityUtils import Role, get_jwt_auth_identity, jwt_auth_required, require_role
from services import service_management

# ........ Functions for job management ...............#
# ......................................................#

serviceblp = Blueprint(
    "Service operations",
    "service",
    url_prefix="/api/service",
    description="Service management operations",
)

servicesblp = Blueprint(
    "Multiple services operations",
    "services",
    url_prefix="/api/services",
    description="Operations on multiple services",
)


@serviceblp.route("/<serviceid>")
class ServiceController(MethodView):
    @serviceblp.response(
        200,
        SchemaWrapper(sla.schema.sla_microservice_schema),
        content_type="application/json",
    )
    @jwt_auth_required()
    def get(self, serviceid):
        """Get service for specific ID

        Requires user to own the service
        ---
        """
        username = get_jwt_auth_identity()
        job = service_management.get_service(serviceid, username)
        if job is not None:
            return json_util.dumps(job)
        else:
            return abort(404, "not found")

    @jwt_auth_required()
    @serviceblp.response(200, content_type="application/json")
    def delete(self, serviceid):
        """Remove service with ID

        Requires user to own the service
        ---
        """
        try:
            username = get_jwt_auth_identity()
            if service_management.delete_service(username, serviceid):
                return {"message": "Job deleted"}
            else:
                abort(500, "Job not deleted")
        except ConnectionError as e:
            abort(500, e)

    @serviceblp.arguments(
        schema=sla.schema.sla_schema, location="json", validate=False, unknown=True
    )
    @serviceblp.response(200, content_type="application/json")
    @jwt_auth_required()
    def put(self, *args, serviceid):
        """Update service with ID

        Requires user to own the service
        ---
        """
        try:
            username = get_jwt_auth_identity()
            job = (request.get_json()["applications"][0])["microservices"][0]
            if "_id" in job:
                del job["_id"]
            result, status = service_management.update_service(username, job, serviceid)
            if status != 200:
                abort(status, result)
            return {}
        except ConnectionError as e:
            abort(404, {"message": e})


@serviceblp.route("/")
class ServiceControllerPost(MethodView):
    @serviceblp.arguments(
        schema=sla.schema.sla_schema, location="json", validate=False, unknown=True
    )
    @serviceblp.response(200, content_type="application/json")
    @jwt_auth_required()
    def post(self, *args, **kwargs):
        """Attach a new service to an application

        Requires user to own the service. Do not specify microserviceID but only AppID.
        ---
        """
        data = request.get_json()
        if data:
            try:
                username = get_jwt_auth_identity()
                result, status = service_management.create_services_of_app(username, data)
                if status != 200:
                    abort(status, result)
                return result
            except Exception as e:
                logging.log(logging.ERROR, e)
                abort(400, {"message": "The given SLA was not formatted correctly"})
        logging.log(logging.ERROR, "POST service no data found")
        abort(404, {"message": "/api/deploy request without a yaml file\n"})


@servicesblp.route("/<appid>")
class MultipleServicesControllerUser(Resource):
    @serviceblp.response(
        200,
        SchemaWrapper(sla.schema.sla_microservices_schema),
        content_type="application/json",
    )
    @jwt_auth_required()
    def get(self, appid):
        username = get_jwt_auth_identity()
        result, status = service_management.user_services(appid, username)
        if status != 200:
            abort(status, result)
        return json_util.dumps(result)


@servicesblp.route("/")
class MultipleServicesController(Resource):
    @serviceblp.response(
        200,
        SchemaWrapper(sla.schema.sla_microservices_schema),
        content_type="application/json",
    )
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def get(self, *args, **kwargs):
        return json_util.dumps(service_management.get_all_services())
