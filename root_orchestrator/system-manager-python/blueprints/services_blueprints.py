from bson import json_util
from flask import request, jsonify, Response
from flask_restful import Resource
from flask_smorest import Blueprint, Api, abort
from flask.views import MethodView

from blueprints.schema_wrapper import SchemaWrapper
from roles.securityUtils import *
import sla.schema
from services import service_management

# ........ Functions for job management ...............#
# ......................................................#
from sla.versioned_sla_parser import SLAFormatError

serviceblp = Blueprint(
    'Service operations', 'service', url_prefix='/api/service',
    description='Service management operations'
)

servicesblp = Blueprint(
    'Multiple services operations', 'services', url_prefix='/api/services',
    description='Operations on multiple services'
)


@serviceblp.route('/<serviceid>')
class ServiceController(MethodView):

    @serviceblp.response(200, SchemaWrapper(sla.schema.sla_microservice_schema), content_type="application/json")
    @jwt_auth_required()
    def get(self, serviceid):
        """Get service for specific ID

        Requires user to own the service
        ---
        """
        username = get_jwt_auth_identity()
        job = service_management.get_service(serviceid, username)
        try:
            return Response(
                response=json_util.dumps(job),
                status=200,
                mimetype="application/json"
            )
        except:
            return {"error": "not found"}, 404

    @jwt_auth_required()
    def delete(self, serviceid):
        """Remove service with ID

        Requires user to own the service
        ---
        """
        try:
            username = get_jwt_auth_identity()
            if service_management.delete_service(username, serviceid):
                resp = jsonify(
                    {"message": "Job deleted"})
                resp.status_code = 200
                return resp
            else:
                resp = jsonify(
                    {"message": "Job could not be deleted"})
                resp.status_code = 200
                return resp
        except ConnectionError as e:
            resp = jsonify({"message": e})
            resp.status_code = 404
            return resp

    @serviceblp.arguments(schema=sla.schema.sla_schema, location="json", validate=False, unknown=True)
    @jwt_auth_required()
    def put(self, serviceid):
        """Update service with ID

        Requires user to own the service
        ---
        """
        try:
            username = get_jwt_auth_identity()
            job = request.get_json()
            if "_id" in job:
                del job['_id']
            service_management.update_service(username, job, serviceid)
            return {}, 200
        except ConnectionError as e:
            resp = jsonify({"message": e})
            resp.status_code = 404
            return resp


@serviceblp.route('/')
class ServiceControllerPost(MethodView):
    @serviceblp.arguments(schema=sla.schema.sla_schema, location="json", validate=False, unknown=True)
    @jwt_auth_required()
    def post(self,stuff):
        """Attach a new service to an application

        Requires user to own the service. Do not specify microserviceID but only AppID.
        ---
        """
        data = request.get_json()
        if data:
            try:
                username = get_jwt_auth_identity()
                return service_management.create_services_of_app(username, data)
            except SLAFormatError:
                return "The given SLA was not formatted correctly", 400
        return "/api/deploy request without a yaml file\n", 400


@servicesblp.route('/<appid>')
class MultipleServicesControllerUser(Resource):

    @serviceblp.response(200, SchemaWrapper(sla.schema.sla_microservices_schema), content_type="application/json")
    @jwt_auth_required()
    def get(self, appid):
        username = get_jwt_auth_identity()
        return Response(
            response=json_util.dumps(service_management.user_services(appid, username)),
            status=200,
            mimetype="application/json"
        )


@servicesblp.route('/')
class MultipleServicesController(Resource):

    @serviceblp.response(200, SchemaWrapper(sla.schema.sla_microservices_schema), content_type="application/json")
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def get(self):
        return Response(
            response=json_util.dumps(service_management.get_all_services()),
            status=200,
            mimetype="application/json"
        )
