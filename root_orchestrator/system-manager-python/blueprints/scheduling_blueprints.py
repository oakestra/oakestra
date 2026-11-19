import logging

from flask import request
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from oakestra_utils.types.statuses import convert_to_status
from resource_abstractor_client import job_operations
from services.instance_management import instance_scale_up_scheduled_handler

schedulingbp = Blueprint("Scheduling", "scheduling-completed", url_prefix="/api/result")
logger = logging.getLogger("system_manager")

auth_schema = {
    "type": "object",
    "properties": {
        "job_id": {"type": "string"},
        "candidate_id": {"type": "string"},
    },
}


@schedulingbp.route("/deploy")
class SchedulingController(MethodView):
    @schedulingbp.arguments(
        schema=auth_schema, location="json", validate=False, unknown=True
    )
    def post(self, *args, **kwargs):
        data = request.get_json()
        logger.info(data)
        job_id = data.get("job_id")
        cluster_id = data.get("candidate_id")
        if cluster_id is None:
            # scheduling failed
            status = data.get("status")
            try:
                new_status = convert_to_status(status)
            except ValueError:
                abort(400, message=f"Unknown status: {status}")
            if new_status is None:
                abort(400, message="Missing status")
            job_operations.update_job_status(job_id, new_status)
        instance_scale_up_scheduled_handler(job_id, cluster_id)
        return "ok"
