"""In-memory stand-in for the resource-abstractor REST API.

It mirrors the behaviour of resource-abstractor/api/v1/{apps,jobs,resources}_blueprint.py
and resource-abstractor/db/*.py closely enough that the system manager cannot tell the
difference: ids are ObjectId strings, PUT on jobs/resources upserts by name, unknown query
parameters are ignored, lookups of missing documents return null (apps) or 404 (jobs,
resources), PUT/PATCH on resources load the body through ResourceSchema (coercing types,
422 on invalid values), and GET/PUT/PATCH on resources dump their response through it.

If the real service changes its contract, update this fake in the same PR, otherwise the
component tests keep passing against behaviour that no longer exists.
"""

import copy
import json
import re
import time
from urllib.parse import parse_qsl, urlparse

from bson import ObjectId
from marshmallow import INCLUDE, Schema, ValidationError, fields
from requests_mock import ANY

CANDIDATES_FRESHNESS_INTERVAL = 30


class ResourceSchema(Schema):
    """Copy of ResourceSchema in resources_blueprint.py. Request bodies are loaded through
    it (so "12" is stored as 12.0 and a non-numeric cpu_percent is a 422) and responses
    are dumped through it, so anything else a caller stored (e.g. "clusterinfo") is not
    returned."""

    _id = fields.String()
    candidate_name = fields.String()
    candidate_location = fields.String()
    ip = fields.String()
    port = fields.String()
    active_nodes = fields.Integer()
    active = fields.Boolean()
    memory = fields.Integer()
    vcpus = fields.Integer()
    vgpus = fields.Integer()
    cpu_percent = fields.Float()
    aggregation_per_architecture = fields.Dict()
    memory_percent = fields.Float()
    gpu_percent = fields.Integer()
    virtualization = fields.List(fields.String(), allow_none=True)
    supported_addons = fields.List(fields.String(), allow_none=True)
    csi_drivers = fields.List(fields.Raw(), allow_none=True)
    architecture = fields.String()
    last_modified_timestamp = fields.Float()


RESOURCE_SCHEMA = ResourceSchema(unknown=INCLUDE)

# CANONICAL_RESOURCES in candidates_db.py: the projection applied to candidate listings.
CANONICAL_RESOURCES = {
    "_id",
    "cpu_percent",
    "vcpus",
    "memory_percent",
    "vram",
    "vram_percent",
    "gpu_temp",
    "gpu_drivers",
    "gpu_percent",
    "vgpus",
    "memory",
    "virtualization",
    "supported_addons",
    "active_nodes",
    "ip",
    "port",
    "candidate_location",
    "candidate_name",
    "cpu_history",
    "memory_history",
    "csi_drivers",
}

INSTANCE_UPDATE_FIELDS = (
    "cpu_percent",
    "memory_percent",
    "publicip",
    "disk",
    "status",
    "logs",
    "worker_id",
    "host_ip",
    "host_port",
    "last_modified_timestamp",
)


class FakeResourceAbstractor:
    def __init__(self, base_url):
        self.base_url = base_url.rstrip("/")
        self.apps = {}
        self.jobs = {}
        self.candidates = {}
        self._outage = False

    # ------------------------------------------------------------------ test API

    def go_down(self):
        """Every subsequent request fails with a 503, like a crashed container."""
        self._outage = True

    def add_candidate(self, **fields):
        """Seed a cluster directly, bypassing gRPC registration."""
        candidate = {"_id": str(ObjectId()), **fields}
        candidate.setdefault("last_modified_timestamp", time.time())
        self.candidates[candidate["_id"]] = candidate
        return copy.deepcopy(candidate)

    def add_app(self, **fields):
        app_id = str(ObjectId())
        app = {"_id": app_id, "applicationID": app_id, "microservices": [], **fields}
        self.apps[app_id] = app
        return copy.deepcopy(app)

    def add_job(self, **fields):
        job_id = str(ObjectId())
        job = {
            "_id": job_id,
            "microserviceID": job_id,
            "instance_list": [],
            "next_instance_progressive_number": 0,
            **fields,
        }
        self.jobs[job_id] = job
        return copy.deepcopy(job)

    def add_service(self, owner="Admin", in_microservices=True, **job_fields):
        """Seed an app + job pair owned by `owner`, linked the way they would be after a
        normal SLA deployment. in_microservices=False leaves the job out of the app's
        microservices list while it still points back at the app."""
        app = self.add_app(userId=owner)
        job = self.add_job(applicationID=app["_id"], **{"job_name": "svc", **job_fields})
        if in_microservices:
            self.apps[app["_id"]]["microservices"].append(job["_id"])
        return job

    def register(self, mocker):
        routes = [
            ("GET", r"/api/v1/applications/?", self._list_apps),
            ("POST", r"/api/v1/applications/?", self._create_app),
            ("GET", r"/api/v1/applications/(?P<id>[^/?]+)", self._get_app),
            ("PATCH", r"/api/v1/applications/(?P<id>[^/?]+)", self._update_app),
            ("DELETE", r"/api/v1/applications/(?P<id>[^/?]+)", self._delete_app),
            ("GET", r"/api/v1/jobs/?", self._list_jobs),
            ("PUT", r"/api/v1/jobs/?", self._upsert_job),
            ("GET", r"/api/v1/jobs/(?P<id>[^/?]+)", self._get_job),
            ("PATCH", r"/api/v1/jobs/(?P<id>[^/?]+)", self._update_job),
            ("DELETE", r"/api/v1/jobs/(?P<id>[^/?]+)", self._delete_job),
            ("GET", r"/api/v1/jobs/(?P<id>[^/?]+)/(?P<n>[^/?]+)", self._get_instance),
            ("PUT", r"/api/v1/jobs/(?P<id>[^/?]+)/(?P<n>[^/?]+)", self._append_instance),
            ("PATCH", r"/api/v1/jobs/(?P<id>[^/?]+)/(?P<n>[^/?]+)", self._update_instance),
            ("DELETE", r"/api/v1/jobs/(?P<id>[^/?]+)/(?P<n>[^/?]+)", self._delete_instance),
            ("GET", r"/api/v1/resources/?", self._list_candidates),
            ("PUT", r"/api/v1/resources/?", self._upsert_candidate),
            ("GET", r"/api/v1/resources/(?P<id>[^/?]+)", self._get_candidate),
            ("PATCH", r"/api/v1/resources/(?P<id>[^/?]+)", self._update_candidate),
        ]
        compiled = [
            (method, re.compile(pattern + r"$"), handler) for method, pattern, handler in routes
        ]

        def dispatch(request, context):
            if self._outage:
                context.status_code = 503
                return b""
            path = urlparse(request.url).path
            for method, pattern, handler in compiled:
                match = pattern.match(path)
                if method == request.method and match:
                    status, body = handler(request, **match.groupdict())
                    context.status_code = status
                    context.headers["Content-Type"] = "application/json"
                    return json.dumps(body).encode()
            context.status_code = 405
            return b""

        mocker.register_uri(ANY, re.compile(re.escape(self.base_url) + r"/"), content=dispatch)

    # ------------------------------------------------------------------ helpers

    @staticmethod
    def _query(request):
        # requests-mock lowercases request.qs, but ids and names are case sensitive.
        return dict(parse_qsl(urlparse(request.url).query))

    @staticmethod
    def _body(request):
        return json.loads(request.body) if request.body else {}

    @staticmethod
    def _matches(doc, query):
        return all(str(doc.get(key)) == value for key, value in query.items())

    def _is_active(self, candidate):
        threshold = time.time() - CANDIDATES_FRESHNESS_INTERVAL
        return candidate.get("last_modified_timestamp", 0) > threshold

    @staticmethod
    def _dump_resource(candidate):
        return RESOURCE_SCHEMA.dump(candidate)

    @staticmethod
    def _load_resource(request):
        """Returns (data, None) or (None, error response), like @arguments(ResourceSchema)."""
        try:
            return RESOURCE_SCHEMA.load(FakeResourceAbstractor._body(request)), None
        except ValidationError as err:
            return None, (422, {"message": "Invalid input", "details": err.messages})

    # ------------------------------------------------------------------ applications

    def _app_filter(self, request):
        allowed = ("application_name", "application_namespace", "userId")
        return {k: v for k, v in self._query(request).items() if k in allowed}

    def _list_apps(self, request):
        query = self._app_filter(request)
        return 200, [copy.deepcopy(a) for a in self.apps.values() if self._matches(a, query)]

    def _create_app(self, request):
        data = self._body(request)
        app_id = str(ObjectId())
        data.pop("_id", None)
        self.apps[app_id] = {**data, "_id": app_id, "applicationID": app_id}
        return 200, copy.deepcopy(self.apps[app_id])

    def _get_app(self, request, id):
        if not ObjectId.is_valid(id):
            return 500, {"message": "invalid ObjectId"}
        app = self.apps.get(id)
        if app is None or not self._matches(app, self._app_filter(request)):
            return 200, None
        return 200, copy.deepcopy(app)

    def _update_app(self, request, id):
        if not ObjectId.is_valid(id):
            return 500, {"message": "invalid ObjectId"}
        data = self._body(request)
        data.pop("_id", None)
        if id not in self.apps:
            return 200, None
        self.apps[id].update(data)
        return 200, copy.deepcopy(self.apps[id])

    def _delete_app(self, request, id):
        if not ObjectId.is_valid(id):
            return 500, {"message": "invalid ObjectId"}
        return 200, self.apps.pop(id, None)

    # ------------------------------------------------------------------ jobs

    def _list_jobs(self, request):
        query = {
            k: v for k, v in self._query(request).items() if k in ("applicationID", "job_name")
        }
        return 200, [copy.deepcopy(j) for j in self.jobs.values() if self._matches(j, query)]

    def _upsert_job(self, request):
        data = self._body(request)
        existing = next(
            (j for j in self.jobs.values() if j.get("job_name") == data.get("job_name")), None
        )
        data.pop("_id", None)
        if existing:
            existing.update(data)
            return 200, copy.deepcopy(existing)
        job_id = str(ObjectId())
        self.jobs[job_id] = {**data, "_id": job_id}
        return 200, copy.deepcopy(self.jobs[job_id])

    def _get_job(self, request, id):
        if not ObjectId.is_valid(id):
            return 400, {"message": "Bad Request"}
        job = self.jobs.get(id)
        if job is None:
            return 404, {"message": "Not Found"}
        return 200, copy.deepcopy(job)

    def _update_job(self, request, id):
        if not ObjectId.is_valid(id):
            return 500, {"message": "invalid ObjectId"}
        data = self._body(request)
        data.pop("_id", None)
        if id not in self.jobs:
            return 200, None
        self.jobs[id].update(data)
        return 200, copy.deepcopy(self.jobs[id])

    def _delete_job(self, request, id):
        if not ObjectId.is_valid(id):
            return 500, {"message": "invalid ObjectId"}
        return 200, self.jobs.pop(id, None)

    def _find_instance(self, job, n):
        for instance in job.get("instance_list", []):
            if str(instance.get("instance_number")) == str(n):
                return instance
        return None

    def _get_instance(self, request, id, n):
        if not ObjectId.is_valid(id):
            return 400, {"message": "Bad Request"}
        job = self.jobs.get(id)
        instance = self._find_instance(job, n) if job else None
        if instance is None:
            return 404, {"message": "Not Found"}
        result = copy.deepcopy(job)
        result["instance_list"] = [copy.deepcopy(instance)]
        return 200, result

    def _append_instance(self, request, id, n):
        if not ObjectId.is_valid(id):
            return 400, {"message": "Bad Request"}
        job = self.jobs.get(id)
        instances = self._body(request).get("instance_list", [])
        if job is None or self._find_instance(job, n) is not None or not instances:
            return 400, {"message": "Instance already exists"}
        job.setdefault("instance_list", []).append(instances[-1])
        return 200, copy.deepcopy(job)

    def _update_instance(self, request, id, n):
        if not ObjectId.is_valid(id):
            return 400, {"message": "Bad Request"}
        job = self.jobs.get(id)
        instance = self._find_instance(job, n) if job else None
        if instance is None:
            return 404, {"message": "Not Found"}
        data = self._body(request)
        for field in INSTANCE_UPDATE_FIELDS:
            instance[field] = data.get(field)
        instance["status_detail"] = data.get("status_detail", "No extra information")
        return 200, copy.deepcopy(job)

    def _delete_instance(self, request, id, n):
        if not ObjectId.is_valid(id):
            return 400, {"message": "Bad Request"}
        job = self.jobs.get(id)
        if job is None:
            return 404, {"message": "Not Found"}
        job["instance_list"] = [
            i for i in job.get("instance_list", []) if str(i.get("instance_number")) != str(n)
        ]
        return 200, copy.deepcopy(job)

    # ------------------------------------------------------------------ resources

    def _list_candidates(self, request):
        query = self._query(request)
        requested = {r.strip() for r in query.get("resources", "").split(",") if r.strip()}
        projection = CANONICAL_RESOURCES | requested
        result = []
        for candidate in self.candidates.values():
            if query.get("active", "").lower() == "true" and not self._is_active(candidate):
                continue
            if (
                "candidate_name" in query
                and candidate.get("candidate_name") != query["candidate_name"]
            ):
                continue
            if "ip" in query and candidate.get("ip") != query["ip"]:
                continue
            enriched = {**candidate, "active": self._is_active(candidate)}
            result.append({k: v for k, v in enriched.items() if k in projection})
        return 200, copy.deepcopy(result)

    def _upsert_candidate(self, request):
        data, error = self._load_resource(request)
        if error:
            return error
        data.pop("_id", None)
        existing = next(
            (
                c
                for c in self.candidates.values()
                if c.get("candidate_name") == data.get("candidate_name")
            ),
            None,
        )
        if existing:
            existing.update(data)
            return 200, self._dump_resource(existing)
        candidate_id = str(ObjectId())
        self.candidates[candidate_id] = {**data, "_id": candidate_id}
        return 200, self._dump_resource(self.candidates[candidate_id])

    def _get_candidate(self, request, id):
        if not ObjectId.is_valid(id):
            return 400, {"message": "Bad Request"}
        candidate = self.candidates.get(id)
        if candidate is None:
            return 404, {"message": "Not Found"}
        # find_candidate_by_id goes through find_candidates' CANONICAL_RESOURCES projection,
        # which drops "active", "last_modified_timestamp" etc. before the schema dump.
        projected = {k: v for k, v in candidate.items() if k in CANONICAL_RESOURCES}
        return 200, self._dump_resource(projected)

    def _update_candidate(self, request, id):
        # Body validation runs in the decorator, before the handler looks at the id.
        data, error = self._load_resource(request)
        if error:
            return error
        if not ObjectId.is_valid(id):
            return 404, {"message": "Not Found"}
        candidate = self.candidates.get(id)
        if candidate is None:
            # The real endpoint dumps None through ResourceSchema, which yields {}.
            return 200, {}
        data.pop("_id", None)
        data.pop("cpu_history", None)
        data.pop("memory_history", None)
        now = time.time()
        data["last_modified_timestamp"] = now
        candidate.setdefault("cpu_history", []).append(
            {"value": data.get("cpu_percent"), "timestamp": now}
        )
        candidate.setdefault("memory_history", []).append(
            {"value": data.get("memory_percent"), "timestamp": now}
        )
        candidate.update(data)
        return 200, self._dump_resource(candidate)
