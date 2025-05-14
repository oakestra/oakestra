import logging

from bson import json_util
from flask import request
from flask.views import MethodView
from flask_restful import Resource
from flask_smorest import Blueprint, abort


# ........ Functions for job management ...............#
# ......................................................#

serviceblp = Blueprint(
    "Service operations",
    "service",
    url_prefix="/api/service",
    description="Service management operations",
)

servicesblp = Blueprint(
    "Multiple services operations",
    "services",
    url_prefix="/api/services",
    description="Operations on multiple services",
)


@serviceblp.route("/<serviceid>")
class ServiceController(MethodView):
    @serviceblp.response(
        200,
        {},
        content_type="application/json",
    )
    def get(self, serviceid):
        """Get service for specific ID

        Requires user to own the service
        ---
        """
        pass
