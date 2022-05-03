from bson import json_util
from flask import request, jsonify, Response
from flask_restful import Resource

from roles.securityUtils import *
from services import service_management


# ........ Functions for job management ...............#
# ......................................................#
from sla.versioned_sla_parser import SLAFormatError


class ServiceController(Resource):

    @staticmethod
    @jwt_auth_required()
    def get(serviceid):
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

    @staticmethod
    @jwt_auth_required()
    def delete(serviceid):
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
            resp = jsonify({"message": e.message})
            resp.status_code = 404
            return resp

    @staticmethod
    @jwt_auth_required()
    def put(jobid):
        try:
            username = get_jwt_auth_identity()
            job = request.get_json()
            if "_id" in job:
                del job['_id']
            service_management.update_service(username, job)
            return {}, 200
        except ConnectionError as e:
            resp = jsonify({"message": e.message})
            resp.status_code = 404
            return resp

    @staticmethod
    @jwt_auth_required()
    def post():
        data = request.get_json()
        if data:
            try:
                username = get_jwt_auth_identity()
                return service_management.create_services_of_app(username, data)
            except SLAFormatError:
                return "The given SLA was not formatted correctly", 400
        return "/api/deploy request without a yaml file\n", 400


class MultipleServicesControllerUser(Resource):

    @staticmethod
    @jwt_auth_required()
    def get(appid):
        username = get_jwt_auth_identity()
        return Response(
            response=json_util.dumps(service_management.user_services(appid,username)),
            status=200,
            mimetype="application/json"
        )


class MultipleServicesController(Resource):

    @staticmethod
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def get():
        return Response(
            response=json_util.dumps(service_management.get_all_services()),
            status=200,
            mimetype="application/json"
        )


