from bson import ObjectId, json_util
from db.clusters_db import (
    create_cluster,
    find_cluster_by_id,
    find_clusters,
    update_cluster_information,
)
from db.clusters_helper import build_filter
from db.jobs_db import find_job_by_id
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import Schema, fields
from werkzeug import exceptions

resourcesblp = Blueprint("Resources Info", "resources_info", url_prefix="/api/v1/resources")


class ResourceFilterSchema(Schema):
    active = fields.Boolean()
    job_id = fields.String()
    cluster_name = fields.String()
    ip = fields.String()


@resourcesblp.route("/")
class AllResourcesController(MethodView):
    @resourcesblp.arguments(ResourceFilterSchema, location="query")
    def get(self, query={}):
        filter = query
        job_id = filter.get("job_id")
        if job_id:
            if ObjectId.is_valid(job_id) is False:
                raise exceptions.BadRequest()

            job = find_job_by_id(job_id)
            if job is None:
                raise exceptions.NotFound()

            cluster_id = job.get("cluster")
            if cluster_id is None:
                raise exceptions.NotFound()

            filter["cluster_id"] = cluster_id
        filter = build_filter(query)

        return json_util.dumps(find_clusters(filter))

    def put(self, *args, **kwargs):
        data = None
        if args and len(args) > 0 and args[0] and type(args[0]) is dict:
            data = args[0]

        if data is None:
            raise exceptions.BadRequest()

        return json_util.dumps(create_cluster(data))


@resourcesblp.route("/<resourceId>")
class ResourceController(MethodView):
    def get(self, resourceId):
        if ObjectId.is_valid(resourceId) is False:
            raise exceptions.BadRequest()

        cluster = find_cluster_by_id(resourceId)
        if cluster is None:
            raise exceptions.NotFound()

        return json_util.dumps(cluster)

    def patch(self, *args, **kwargs):
        resource_id = kwargs["resourceId"]
        data = None
        if args and len(args) > 0 and args[0] and type(args[0]) is dict:
            data = args[0]

        if data is None:
            raise exceptions.BadRequest()

        if ObjectId.is_valid(resource_id) is False:
            raise exceptions.BadRequest()

        return json_util.dumps(update_cluster_information(resource_id, data))
