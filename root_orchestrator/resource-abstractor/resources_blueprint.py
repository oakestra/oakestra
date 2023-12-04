from flask_smorest import Blueprint
from flask.views import MethodView
from mongodb_client import mongo_get_all_clusters, mongo_get_cluster_by_id
from marshmallow import Schema, fields
from bson.objectid import ObjectId
from werkzeug.exceptions import NotFound, BadRequest

resourceblp = Blueprint(
    'Resource Info', 'resource info', url_prefix='/api/info'
)

def get_resource_blueprint():
    return resourceblp

class ResourceSchema(Schema):
    id = fields.String(attribute='_id')
    cluster_name = fields.String()
    cluster_location = fields.String()
    ip = fields.String()
    port = fields.String()
    active_nodes = fields.Integer()

    memory_in_mb = fields.Integer()
    total_cpu_cores = fields.Integer()
    total_gpu_cores = fields.Integer()
    virtualization = fields.List(fields.String())

@resourceblp.route('/')
class AllResourcesController(MethodView):

    @resourceblp.response(200, ResourceSchema(many=True), content_type="application/json")
    def get(self, *args, **kwargs):
        # TODO: support pagination
        clusters = list(mongo_get_all_clusters())
        return clusters
    
@resourceblp.route('/<resourceId>')
class ResourceController(MethodView):

    @resourceblp.response(200, ResourceSchema(), content_type="application/json")
    def get(self, resourceId, *args, **kwargs):
        if ObjectId.is_valid(resourceId) is False:
            raise BadRequest()
        
        cluster = mongo_get_cluster_by_id(resourceId)
        if cluster is None:
            raise NotFound()

        return cluster