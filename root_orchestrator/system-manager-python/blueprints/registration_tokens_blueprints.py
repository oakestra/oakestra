import logging
import os

import requests
from ext_requests.cluster_requests import cluster_push_worker_token
from ext_requests.registration_tokens_db import (
    TOKEN_TYPE_CLUSTER,
    create_registration_token,
    generate_token,
)
from flask import request
from flask_jwt_extended import jwt_required
from flask_restful import Resource
from flask_smorest import abort
from resource_abstractor_client import candidate_operations
from roles.securityUtils import Role, get_jwt_auth_identity, require_any_role

from blueprints.jwt_wrapper import BlueprintExt

logger = logging.getLogger("system_manager")

tokensbp = BlueprintExt("Registration tokens", "tokens", url_prefix="/api/tokens")

cluster_token_schema = {
    "type": "object",
    "properties": {
        "ttl_minutes": {"type": "integer", "minimum": 1},
    },
}

worker_token_schema = {
    "type": "object",
    "properties": {
        "cluster_id": {"type": "string"},
        "ttl_minutes": {"type": "integer", "minimum": 1},
    },
    "required": ["cluster_id"],
}


def _root_public_address() -> str:
    # ROOT_PUBLIC_ADDRESS is authoritative; the forwarded host is a
    # convenience fallback for the suggested command only.
    address = os.environ.get("ROOT_PUBLIC_ADDRESS")
    if address:
        return address
    forwarded = request.headers.get("X-Forwarded-Host")
    if forwarded:
        return forwarded.split(":")[0]
    return request.host.split(":")[0]


@tokensbp.route("/cluster")
class ClusterRegistrationTokenController(Resource):
    @tokensbp.arguments(schema=cluster_token_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_any_role(Role.ADMIN, Role.INF_Provider)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}

        token, expiry_date = create_registration_token(
            token_type=TOKEN_TYPE_CLUSTER,
            created_by=get_jwt_auth_identity(),
            ttl_minutes=content.get("ttl_minutes"),
        )

        root_address = _root_public_address()
        return {
            "token": token,
            "expires_at": expiry_date.isoformat(),
            "root_address": root_address,
            "root_port": 443,
            "suggested_command": (
                f"CLUSTER_REGISTRATION_TOKEN={token} "
                f"SYSTEM_MANAGER_URL={root_address} "
                "CLUSTER_NAME=<name> "
                "./StartOakestraCluster.sh"
            ),
        }


@tokensbp.route("/worker")
class WorkerRegistrationTokenController(Resource):
    @tokensbp.arguments(schema=worker_token_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_any_role(Role.ADMIN, Role.INF_Provider)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        cluster_id = content.get("cluster_id")
        if not cluster_id:
            abort(400, message="cluster_id is required")

        cluster = candidate_operations.get_candidate_by_id(cluster_id)
        if cluster is None:
            abort(404, message=f"Cluster {cluster_id} not found")

        # The token hash lives only at the target cluster, which validates
        # worker redemptions locally — the root keeps no copy.
        token, token_hash, expiry_date = generate_token(content.get("ttl_minutes"))
        try:
            response = cluster_push_worker_token(cluster, token_hash, expiry_date.isoformat())
        except requests.exceptions.RequestException as e:
            logger.error(f"Worker token delivery to cluster {cluster_id} failed: {e}")
            abort(502, message="Could not deliver token to cluster")
        if response.status_code not in (200, 204):
            logger.error(
                f"Worker token delivery to cluster {cluster_id} rejected: "
                f"{response.status_code} - {response.text}"
            )
            abort(502, message="Could not deliver token to cluster")

        cluster_address = cluster.get("ip")
        cluster_port = cluster.get("port")
        return {
            "token": token,
            "expires_at": expiry_date.isoformat(),
            "cluster_address": cluster_address,
            "cluster_port": cluster_port,
            "suggested_command": (
                f"sudo NodeEngine -a {cluster_address} -p {cluster_port} -s --token {token}"
            ),
        }
