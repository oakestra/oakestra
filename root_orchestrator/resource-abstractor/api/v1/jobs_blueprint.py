from bson import ObjectId, json_util
from db.jobs_db import find_all_jobs, find_job_by_id, update_job
from flask.views import MethodView
from flask_smorest import Blueprint
from werkzeug import exceptions

jobsblp = Blueprint("Jobs Api", "jobs_api", url_prefix="/api/v1/jobs")


@jobsblp.route("/")
class AllJobsController(MethodView):
    def get(self):
        return json_util.dumps(find_all_jobs())


@jobsblp.route("/<jobId>")
class JobController(MethodView):
    def get(self, jobId):
        if ObjectId.is_valid(jobId) is False:
            raise exceptions.BadRequest()

        job = find_job_by_id(jobId)
        if job is None:
            raise exceptions.NotFound()

        return json_util.dumps(job)

    def patch(self, *args, **kwargs):
        job_id = kwargs["jobId"]
        data = None
        if args and len(args) > 0 and args[0] and type(args[0]) is dict:
            data = args[0]

        if data is None:
            raise exceptions.BadRequest()

        if ObjectId.is_valid(job_id) is False:
            raise exceptions.BadRequest()

        return json_util.dumps(update_job(job_id, data))
