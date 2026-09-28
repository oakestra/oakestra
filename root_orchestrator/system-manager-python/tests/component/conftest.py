"""Harness that boots the real system manager with every external dependency faked.

The system manager wires itself up at import time (fetches the JWT public key, connects
to Mongo, creates the admin user, starts the gRPC server), so the environment and the
fakes have to be in place *before* `import system_manager`. That is why the env vars are
set at module level here and the import happens inside a session fixture.

Outbound HTTP is intercepted at the transport level with requests-mock, so a test can
never reach a real service by accident (see the `http` fixture).
"""

import copy
import importlib.util
import json
import os
import re
import socket
from datetime import timedelta
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock, patch
from urllib.parse import urlparse

import grpc
import mongomock
import pytest
import requests
import requests_mock
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from requests_mock import ANY

from tests.component.fakes.resource_abstractor import FakeResourceAbstractor

RESOURCE_ABSTRACTOR = "http://resource-abstractor:11011"
SCHEDULER = "http://root-scheduler:10004"
NET_PLUGIN = "http://net-plugin:10010"
JWT_GENERATOR = "http://jwt-generator:10011"

JWT_GENERATOR_SOURCE = Path(__file__).parents[3] / "jwt-generator" / "jwt_generator.py"

ADMIN_USERNAME = "Admin"
ADMIN_PASSWORD = "Admin"

APP_NO_SERVICES = {
    "sla_version": "v2.0",
    "customerID": "x",
    "applications": [
        {
            "application_name": "AppOne",
            "application_namespace": "test",
            "application_desc": "d",
            "microservices": [],
        }
    ],
}


def _free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def pem_keypair():
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    private_pem = key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode()
    public_pem = (
        key.public_key()
        .public_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PublicFormat.SubjectPublicKeyInfo,
        )
        .decode()
    )
    return private_pem, public_pem


JWT_PRIVATE_KEY, JWT_PUBLIC_KEY = pem_keypair()
GRPC_PORT = _free_port()

os.environ.update(
    {
        "ROOT_MONGO_URL": "localhost",
        "ROOT_MONGO_PORT": "10007",
        "RESOURCE_ABSTRACTOR_URL": "resource-abstractor",
        "RESOURCE_ABSTRACTOR_PORT": "11011",
        "ROOT_SCHEDULER_URL": "root-scheduler",
        "ROOT_SCHEDULER_PORT": "10004",
        "NET_PLUGIN_URL": "net-plugin",
        "NET_PLUGIN_PORT": "10010",
        "JWT_GENERATOR_URL": "jwt-generator",
        "JWT_GENERATOR_PORT": "10011",
        "MY_PORT_GRPC": str(GRPC_PORT),
        "MAIL_USER": "",
        "MAIL_PASSWORD": "",
        # Read by jwt_generator.py so it signs with a key the tests know.
        "JWT_PRIVATE_KEY": JWT_PRIVATE_KEY,
        "JWT_PUBLIC_KEY": JWT_PUBLIC_KEY,
    }
)


# ---------------------------------------------------------------------------- boot


@pytest.fixture(scope="session")
def jwt_generator_app():
    """The real jwt_generator Flask app, used in-process as the system manager's
    token service. This doubles as a contract test between the two components."""
    spec = importlib.util.spec_from_file_location("jwt_generator_app", JWT_GENERATOR_SOURCE)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.app


def _forward_to(flask_app):
    client = flask_app.test_client()

    def forward(request, context):
        response = client.open(
            urlparse(request.url).path,
            method=request.method,
            data=request.body,
            headers={"Content-Type": request.headers.get("Content-Type", "")},
        )
        context.status_code = response.status_code
        context.headers["Content-Type"] = response.headers.get("Content-Type", "")
        return response.get_data()

    return forward


@pytest.fixture(scope="session")
def mongo_client():
    return mongomock.MongoClient()


@pytest.fixture(scope="session")
def sm(jwt_generator_app, mongo_client):
    """The imported system_manager module (Flask app + running gRPC server)."""
    from ext_requests import mongodb_client

    def fake_pymongo(app, uri):
        return SimpleNamespace(db=mongo_client[uri.rsplit("/", 1)[-1]])

    with patch.object(mongodb_client, "PyMongo", fake_pymongo), requests_mock.Mocker() as m:
        m.register_uri(
            ANY, re.compile(re.escape(JWT_GENERATOR)), content=_forward_to(jwt_generator_app)
        )
        import system_manager

    # Deliberately not TESTING mode: that re-raises view exceptions into the test, while a
    # real client gets a 500. Component tests assert what a client would see.
    system_manager.app.config["PROPAGATE_EXCEPTIONS"] = False
    return system_manager


@pytest.fixture(scope="session")
def app(sm):
    return sm.app


# ---------------------------------------------------------------------------- per test state


@pytest.fixture(scope="session")
def seeded_root(sm):
    """The admin user and root organization as create_admin() leaves them. Seeded once:
    create_admin() hashes a password, which is too slow to repeat for every test."""
    import ext_requests.mongodb_client as db
    from ext_requests.user_db import create_admin

    create_admin()
    return list(db.mongo_users.find()), list(db.mongo_organization.find())


@pytest.fixture(autouse=True)
def mongo(seeded_root):
    """Fresh users/organizations for every test, seeded like a freshly started root."""
    import ext_requests.mongodb_client as db

    users, organizations = seeded_root
    db.mongo_users.delete_many({})
    db.mongo_organization.delete_many({})
    db.mongo_users.insert_many(copy.deepcopy(users))
    db.mongo_organization.insert_many(copy.deepcopy(organizations))
    return SimpleNamespace(users=db.mongo_users, organizations=db.mongo_organization)


@pytest.fixture
def resource_abstractor():
    return FakeResourceAbstractor(RESOURCE_ABSTRACTOR)


class Outbound:
    """Read-only view on the HTTP requests the system manager sent to its dependencies."""

    def __init__(self, mocker):
        self._mocker = mocker

    def calls(self, base_url, method=None, path=None):
        result = []
        for request in self._mocker.request_history:
            if not request.url.startswith(base_url):
                continue
            if method and request.method != method:
                continue
            parsed = urlparse(request.url)
            if path and parsed.path.rstrip("/") != path.rstrip("/"):
                continue
            body = json.loads(request.body) if request.body else None
            result.append(SimpleNamespace(method=request.method, path=parsed.path, json=body))
        return result

    def scheduler(self, **kwargs):
        return self.calls(SCHEDULER, **kwargs)

    def net_plugin(self, **kwargs):
        return self.calls(NET_PLUGIN, **kwargs)

    def cluster(self, cluster, **kwargs):
        return self.calls(cluster.base_url, **kwargs)


def route(http, base_url, **response):
    """Answer every request under `base_url` with `response` (requests-mock kwargs).
    Later registrations win, so this also overrides a default route."""
    http.register_uri(ANY, re.compile(re.escape(base_url) + "/"), **response)


@pytest.fixture(autouse=True)
def http(sm, jwt_generator_app, resource_abstractor):
    """Transport-level mock for all outbound HTTP, pre-wired with the default fakes.

    A request to an address no fake answers behaves like a dead host (ConnectionError, so
    the production error handling runs as it would in a real outage) and fails the test at
    teardown: every dependency a test talks to has to be declared.
    """
    unexpected = []

    def refuse(request, context):
        unexpected.append(f"{request.method} {request.url}")
        raise requests.ConnectionError(f"no fake is listening on {request.url}")

    with requests_mock.Mocker() as m:
        # Registered first, so it only matches when nothing more specific does.
        m.register_uri(ANY, re.compile(".*"), content=refuse)
        route(m, JWT_GENERATOR, content=_forward_to(jwt_generator_app))
        resource_abstractor.register(m)
        route(m, SCHEDULER, status_code=200, json={})
        route(m, NET_PLUGIN, status_code=200, json={})
        yield m

    assert not unexpected, f"requests to undeclared dependencies: {unexpected}"


@pytest.fixture
def outbound(http):
    return Outbound(http)


@pytest.fixture
def net_plugin_fails(http):
    """Make every network plugin call fail with a 500."""
    route(http, NET_PLUGIN, status_code=500, json={})


@pytest.fixture
def scheduler_down(http):
    route(http, SCHEDULER, exc=requests.ConnectionError)


@pytest.fixture
def jwt_generator_down(http):
    route(http, JWT_GENERATOR, exc=requests.ConnectionError)


@pytest.fixture(autouse=True)
def synchronous_threads(monkeypatch):
    """Scale-up hands the scheduler request to a thread; run it inline so assertions
    about outbound calls are deterministic."""
    from services import instance_management

    class InlineThread:
        def __init__(self, group=None, target=None, args=(), kwargs=None, **_):
            self._target, self._args, self._kwargs = target, args, kwargs or {}

        def start(self):
            self._target(*self._args, **self._kwargs)

    # Swap the module's reference only; patching threading.Thread itself would also hit
    # the gRPC server's thread pool.
    monkeypatch.setattr(instance_management, "threading", SimpleNamespace(Thread=InlineThread))


@pytest.fixture(autouse=True)
def smtp(monkeypatch):
    import mail.mail

    server = MagicMock(name="SMTP_SSL")
    monkeypatch.setattr(mail.mail.smtplib, "SMTP_SSL", server)
    return server


@pytest.fixture(autouse=True)
def fast_reachability_probe(sm, monkeypatch):
    # Production retries for 30 s so a slow-booting cluster manager can register. In tests
    # an unreachable cluster must fail on the first attempt.
    monkeypatch.setattr(sm, "CLUSTER_REACHABILITY_TOTAL_WINDOW", 0)


# ---------------------------------------------------------------------------- clusters


class FakeCluster:
    """A cluster manager endpoint. Accepts every request with a 200 unless told otherwise."""

    def __init__(self, http, ip, port, name):
        self.ip, self.port, self.name = ip, port, name
        host = f"[{ip}]" if ":" in ip else ip
        self.base_url = f"http://{host}:{port}"
        self.id = None
        self._http = http
        self.respond(200)

    def respond(self, status_code):
        route(self._http, self.base_url, status_code=status_code, json={})

    def go_down(self):
        route(self._http, self.base_url, exc=requests.ConnectionError)


@pytest.fixture
def make_cluster(http, resource_abstractor):
    """Create a reachable cluster manager and, unless registered=False, seed it in the
    resource abstractor as a fresh (active) candidate."""
    counter = iter(range(1, 255))

    def make(registered=True, ip=None, port=10100, name=None, **candidate_fields):
        n = next(counter)
        cluster = FakeCluster(http, ip or f"10.0.0.{n}", port, name or f"cluster{n}")
        if registered:
            candidate = resource_abstractor.add_candidate(
                candidate_name=cluster.name,
                candidate_location="48.1,11.5,1000",
                ip=cluster.ip,
                port=str(port),
                **candidate_fields,
            )
            cluster.id = candidate["_id"]
        return cluster

    return make


# ---------------------------------------------------------------------------- grpc


@pytest.fixture(scope="session")
def grpc_stub(sm):
    from proto.clusterRegistration_pb2_grpc import register_clusterStub

    # grpc honours http(s)_proxy, which would route localhost through a proxy on CI
    # runners and dev machines that set one.
    channel = grpc.insecure_channel(
        f"127.0.0.1:{GRPC_PORT}", options=[("grpc.enable_http_proxy", 0)]
    )
    grpc.channel_ready_future(channel).result(timeout=10)
    yield register_clusterStub(channel)
    channel.close()


def cs2_message(
    cluster, network_component_port=44444, cluster_info=None, cluster_location="48.1,11.5,1000"
):
    """The CS2 step of the registration handshake, as a cluster manager would send it."""
    from proto.clusterRegistration_pb2 import CS2Message, KeyValue

    return CS2Message(
        manager_port=cluster.port,
        network_component_port=network_component_port,
        cluster_name=cluster.name,
        cluster_info=[KeyValue(key="k1", value="v1")] if cluster_info is None else cluster_info,
        cluster_location=cluster_location,
        cluster_address=cluster.ip,
    )


# ---------------------------------------------------------------------------- auth


@pytest.fixture
def client(app):
    return app.test_client()


def bearer(token):
    return {"Authorization": f"Bearer {token}"}


@pytest.fixture
def login(client):
    def do_login(username=ADMIN_USERNAME, password=ADMIN_PASSWORD, **extra):
        response = client.post(
            "/api/auth/login", json={"username": username, "password": password, **extra}
        )
        assert response.status_code == 200, response.get_data(as_text=True)
        return response.get_json()

    return do_login


@pytest.fixture
def admin_headers(login):
    return bearer(login()["token"])


@pytest.fixture
def root_org_id(mongo):
    return str(mongo.organizations.find_one({"name": "root"})["_id"])


@pytest.fixture
def admin_user_id(mongo):
    return str(mongo.users.find_one({"name": ADMIN_USERNAME})["_id"])


@pytest.fixture
def create_user(client, admin_headers, login):
    """Register a user in the root organization through the API and return its login headers."""

    def create(name, roles=("Application_Provider",), password="s3cret-pw", email=None):
        response = client.post(
            "/api/auth/register",
            json={
                "name": name,
                "password": password,
                "email": email or f"{name}@example.com",
                "created_at": "01/01/2026 10:00",
                "roles": list(roles),
            },
            headers=admin_headers,
        )
        assert response.status_code == 201, response.get_data(as_text=True)
        return bearer(login(name, password)["token"])

    return create


@pytest.fixture
def mint_token(jwt_generator_app, root_org_id):
    """Sign an admin access token with the key the system manager trusts, for tests that
    need claims or lifetimes the login flow never produces."""
    from flask_jwt_extended import create_access_token

    def mint(expires_delta=timedelta(minutes=5), **claims):
        additional = {
            "user": ADMIN_USERNAME,
            "roles": ["Admin"],
            "organization": root_org_id,
            **claims,
        }
        with jwt_generator_app.app_context():
            return create_access_token(
                identity=ADMIN_USERNAME, additional_claims=additional, expires_delta=expires_delta
            )

    return mint
