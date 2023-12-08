from flask_smorest import Blueprint
from flask.views import MethodView
from marshmallow import Schema, fields
from db import find_all_jobs, find_job_by_id
from bson.objectid import ObjectId
from werkzeug import exceptions

jobsblp = Blueprint(
    'Jobs Api', url_prefix='/jobs'
)

class JobSchema(Schema):
    id = fields.String(attribute='_id')
    system_job_id = fields.String()
    job_name = fields.String()
    status = fields.String()
    replicas = fields.Integer()
    instance_list = fields.List(fields.Dict())
    service_ip_list = fields.List(fields.Dict())
    last_modified_timestamp = fields.Float()

@jobsblp.route('/')
class AllJobsController(MethodView):

    @jobsblp.response(200, JobSchema(many=True), content_type="application/json")
    def get(self, args, *kwargs):
        # TODO: support pagination
        return list(find_all_jobs())

@jobsblp.route('/<jobId>')
class JobController(MethodView):

    @jobsblp.response(200, JobSchema(), content_type="application/json")
    def get(self, jobId, args, *kwargs):
        if ObjectId.is_valid(jobId) is False:
            raise exceptions.BadRequest()
        
        job = find_job_by_id(jobId)
        if job is None:
            raise exceptions.NotFound()

        return job
