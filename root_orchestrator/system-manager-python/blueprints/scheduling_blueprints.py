from flask import request
from flask.views import MethodView
from flask_smorest import Blueprint
from oakestra_logging import get_logger
from oakestra_utils.types.statuses import convert_to_status
from resource_abstractor_client import job_operations
from services.instance_management import instance_scale_up_scheduled_handler

schedulingbp = Blueprint("Scheduling", "scheduling-completed", url_prefix="/api/result")
logger = get_logger(__name__)

auth_schema = {
    "type": "object",
    "properties": {
        "job_id": {"type": "string"},
        "candidate_id": {"type": "string"},
    },
}


@schedulingbp.route("/deploy")
class SchedulingController(MethodView):
    @schedulingbp.arguments(schema=auth_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        data = request.get_json()
        job_id = data.get("job_id")
        cluster_id = data.get("candidate_id")
        logger.info(
            "Received scheduling result",
            event_name="scheduling.result.received",
            job_id=job_id,
            cluster_id=cluster_id,
            status=data.get("status"),
        )
        if cluster_id is None:
            # scheduling failed
            status = data.get("status")
            job_operations.update_job_status(job_id, convert_to_status(status))
        instance_scale_up_scheduled_handler(job_id, cluster_id)
        return "ok"
