import logging

from flask import request
from flask.views import MethodView
from flask_smorest import Blueprint
from services.instance_management import instance_scale_up_scheduled_handler

schedulingbp = Blueprint("Scheduling", "scheduling-completed", url_prefix="/api/result")

auth_schema = {
    "type": "object",
    "properties": {
        "job_id": {"type": "string"},
        "cluster_id": {"type": "string"},
    },
}


@schedulingbp.route("/deploy")
class SchedulingController(MethodView):
    @schedulingbp.arguments(schema=auth_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        data = request.get_json()
        logging.log(logging.INFO, data)
        job_id = data.get("job_id")
        cluster_id = data.get("cluster_id")
        instance_scale_up_scheduled_handler(job_id, cluster_id)
        return "ok"
