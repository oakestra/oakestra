import logging

from ext_requests.cluster_certificates import (
    generate_worker_cert,
    get_root_ca_pem,
    sign_worker_csr,
)
from flask import Response, request
from flask.views import MethodView
from flask_smorest import Blueprint, abort

logger = logging.getLogger("cluster_manager")

certbp = Blueprint(
    "Cluster certificates",
    "cluster_certificates",
    url_prefix="/api/certs",
    description="Worker certificate provisioning (internal gateway only)",
)


generate_worker_schema = {
    "type": "object",
    "properties": {
        "common_name": {"type": "string"},
        "alt_names": {"type": "array", "items": {"type": "string"}},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["common_name"],
}

sign_csr_schema = {
    "type": "object",
    "properties": {
        "csr": {"type": "string"},
        "valid_days": {"type": "integer", "minimum": 1},
    },
    "required": ["csr"],
}


@certbp.route("/ca.crt")
class ClusterRootCaController(MethodView):
    def get(self):
        try:
            pem = get_root_ca_pem()
        except FileNotFoundError:
            abort(503, message="Root CA not available on this cluster")
        return Response(
            pem,
            mimetype="application/x-pem-file",
            headers={"Content-Disposition": "attachment; filename=ca.crt"},
        )


@certbp.route("/generate-worker")
class ClusterGenerateWorkerController(MethodView):
    @certbp.arguments(schema=generate_worker_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        common_name = content.get("common_name")
        if not common_name:
            abort(400, message="common_name is required")

        alt_names = content.get("alt_names") or None
        valid_days = int(content.get("valid_days") or 365)

        try:
            private_pem, fullchain_pem = generate_worker_cert(
                common_name=common_name, alt_names=alt_names, valid_days=valid_days
            )
            root_ca_pem = get_root_ca_pem()
        except FileNotFoundError:
            abort(409, message="Cluster intermediate CA material is not initialized")
        except ValueError as e:
            abort(400, message=f"Worker certificate generation failed: {str(e)}")

        return {
            "private_key": private_pem,
            "certificate": fullchain_pem,
            "root_ca": root_ca_pem,
        }


@certbp.route("/sign")
class ClusterSignCsrController(MethodView):
    @certbp.arguments(schema=sign_csr_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        csr_pem = content.get("csr") or ""
        if not csr_pem.strip():
            abort(400, message="csr is required")

        try:
            fullchain_pem = sign_worker_csr(
                csr_pem=csr_pem,
                valid_days=int(content.get("valid_days") or 365),
            )
        except FileNotFoundError:
            abort(409, message="Cluster intermediate CA material is not initialized")
        except ValueError as e:
            abort(400, message=f"Invalid CSR format: {str(e)}")

        return {"certificate": fullchain_pem}
