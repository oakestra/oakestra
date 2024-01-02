from bson.objectid import ObjectId
from db.jobs_db import find_all_jobs, find_job_by_id, update_job
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import Schema, fields
from werkzeug import exceptions

jobsblp = Blueprint("Jobs Api", "jobs_api", url_prefix="/api/v1/jobs")


class JobSchema(Schema):
    id = fields.String(attribute="_id")
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


@jobsblp.route("/")
class AllJobsController(MethodView):
    @jobsblp.response(200, JobSchema(many=True), content_type="application/json")
    def get(self):
        # TODO: support pagination
        return list(find_all_jobs())


@jobsblp.route("/<jobId>")
class JobController(MethodView):
    @jobsblp.response(200, JobSchema(), content_type="application/json")
    def get(self, jobId):
        if ObjectId.is_valid(jobId) is False:
            raise exceptions.BadRequest()

        job = find_job_by_id(jobId)
        if job is None:
            raise exceptions.NotFound()

        return job

    @jobsblp.arguments(JobSchema(), location="json")
    @jobsblp.response(200, content_type="application/json")
    def patch(self, *args, **kwargs):
        job_id = kwargs["jobId"]
        data = None
        if args and len(args) > 0 and args[0] and type(args[0]) is dict:
            data = args[0]

        if data is None:
            raise exceptions.BadRequest()

        if ObjectId.is_valid(job_id) is False:
            raise exceptions.BadRequest()

        update_job(job_id, data)
