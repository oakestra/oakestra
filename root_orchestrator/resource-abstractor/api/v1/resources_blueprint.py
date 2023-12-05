from flask_smorest import Blueprint
from flask.views import MethodView
from mongodb_client import find_all_clusters, find_cluster_by_id, find_active_clusters
from marshmallow import Schema, fields
from bson.objectid import ObjectId
from werkzeug.exceptions import NotFound, BadRequest

resourceblp = Blueprint(
    'Resource Info', 'resource info', url_prefix='/resources'
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
    last_modified_timestamp = fields.Double()

class ResourceFilterSchema(Schema):
    active = fields.Boolean()

@resourceblp.route('/')
class AllResourcesController(MethodView):

    @resourceblp.arguments(ResourceFilterSchema, location='query')
    @resourceblp.response(200, ResourceSchema(many=True), content_type="application/json")
    def get(self, args, *kwargs):
        # TODO: support pagination
        active = args.get('active')
        if active:
            return list(find_active_clusters())
        
        return list(find_all_clusters())
    
@resourceblp.route('/<resourceId>')
class ResourceController(MethodView):

    @resourceblp.response(200, ResourceSchema(), content_type="application/json")
    def get(self, resourceId, args, *kwargs):
        if ObjectId.is_valid(resourceId) is False:
            raise BadRequest()
        
        cluster = find_cluster_by_id(resourceId)
        if cluster is None:
            raise NotFound()

        return cluster
    