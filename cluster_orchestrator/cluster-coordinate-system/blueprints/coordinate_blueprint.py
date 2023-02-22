from flask_smorest import Blueprint, abort, Api
from flask.views import MethodView
from models.query import *
from geolite.geolitedb import *
from bson import json_util
import string

coordinateblp = Blueprint(
    'Coordinate query', 'coordinates', url_prefix='/api/coordinate',
    description='Get coordinated from IP address'
)


@coordinateblp.route('/')
class ApplicationController(MethodView):

    @coordinateblp.response(200, QueryResponseSchema, content_type="application/json")
    @coordinateblp.arguments(QueryParameterSchema, location="query",
                             description="Comma-separated list of ip addresses.", required=True)
    def get(self,args):
        try:
            ip_addresses = request.args.get('ip_addresses', '').split(',')
            return json_util.dumps(Geolite().query_geolocation_for_ips(ip_addresses))
        except Exception as e:
            logging.error(e)
            return abort(500, {"message": e})
