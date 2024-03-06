from bson.objectid import ObjectId
from db import jobs_db
from db.jobs_helper import build_filter
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import INCLUDE, Schema, fields
from werkzeug import exceptions

jobsblp = Blueprint("Jobs Api", "jobs_api", url_prefix="/api/v1/jobs")


class JobSchema(Schema):
    _id = fields.String()
    system_job_id = fields.String()
    job_name = fields.String()
    service_name = fields.String()
    code = fields.String()
    cmd = fields.List(fields.String())
    status = fields.String()
    replicas = fields.Integer()
    instance_list = fields.List(fields.Dict())
    service_ip_list = fields.List(fields.Dict())
    last_modified_timestamp = fields.Float()
    cluster = fields.String()
    status = fields.String()


class JobFilterSchema(Schema):
    instance_number = fields.Integer()


@jobsblp.route("/")
class AllJobsController(MethodView):
    @jobsblp.response(200, JobSchema(many=True), content_type="application/json")
    def get(self):
        return list(jobs_db.find_all_jobs())

    @jobsblp.arguments(JobSchema(unknown=INCLUDE), location="json")
    @jobsblp.response(201, JobSchema, content_type="application/json")
    def post(self, *args, **kwargs):
        data = args[0]

        return jobs_db.create_job(data)

    @jobsblp.arguments(JobSchema(unknown=INCLUDE), location="json")
    @jobsblp.response(200, JobSchema(many=True), content_type="application/json")
    def put(self, *args, **kwargs):
        data = args[0]

        return jobs_db.create_update_job(data)


@jobsblp.route("/<jobid>")
class JobController(MethodView):
    @jobsblp.arguments(JobFilterSchema, location="query")
    @jobsblp.response(200, JobSchema, content_type="application/json")
    def get(self, jobid, query={}):
        if ObjectId.is_valid(jobid) is False:
            raise exceptions.BadRequest()

        filter = build_filter(query)
        job = jobs_db.find_job_by_id(jobid, filter)
        if job is None:
            raise exceptions.NotFound()

        return job

    @jobsblp.arguments(JobSchema(unknown=INCLUDE), location="json")
    @jobsblp.response(200, JobSchema, content_type="application/json")
    def patch(self, jobid, *args, **kwargs):
        data = args[0]

        if ObjectId.is_valid(jobid) is False:
            raise exceptions.BadRequest()

        return jobs_db.update_job(jobid, data)

    def delete(self, jobid, *args, **kwargs):
        if ObjectId.is_valid(jobid) is False:
            raise exceptions.BadRequest()

        return jobs_db.delete_job(jobid)


@jobsblp.route("/<jobid>/<instance>")
class JobInstanceController(MethodView):
    @jobsblp.arguments(JobSchema(unknown=INCLUDE), location="json")
    @jobsblp.response(200, JobSchema, content_type="application/json")
    def patch(self, jobid, instance, *args, **kwargs):
        data = args[0]

        if ObjectId.is_valid(jobid) is False:
            raise exceptions.BadRequest()

        return jobs_db.update_job_instance(jobid, instance, data)
