"""Shared fixtures for resource_abstractor_client tests.

Every test talks to a fake abstractor via httpx.MockTransport rather than mocking
resource_abstractor_client itself, so what's actually asserted is the wire form each
facade function produces - method, path, query string, body - the same thing that has
to agree with both go_resource_abstractor and its Python predecessor.
"""

import os

import httpx
import pytest

os.environ.setdefault("RESOURCE_ABSTRACTOR_URL", "fake-abstractor")
os.environ.setdefault("RESOURCE_ABSTRACTOR_PORT", "11011")

from resource_abstractor_client import client_helper  # noqa: E402

FAKE_ADDR = "http://fake-abstractor:11011"


class RequestRecorder:
    """Records every request sent through the mocked httpx transport and lets a test
    script the responses to hand back, in order."""

    def __init__(self):
        self.requests: list[httpx.Request] = []
        self._responses: list[httpx.Response] = []

    def queue(self, response: httpx.Response) -> None:
        self._responses.append(response)

    def __call__(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        if self._responses:
            return self._responses.pop(0)
        # Default: echo an empty document back, good enough for tests that only
        # care about the outgoing request.
        return httpx.Response(200, json={})

    @property
    def last(self) -> httpx.Request:
        return self.requests[-1]


@pytest.fixture
def recorder(monkeypatch):
    """Points client_helper at a fake abstractor whose responses a test controls via
    recorder.queue(...), and whose received requests are inspectable via
    recorder.requests / recorder.last.
    """
    rec = RequestRecorder()
    fake_client = client_helper.Client(
        base_url=FAKE_ADDR,
        httpx_args={"transport": httpx.MockTransport(rec)},
    )
    monkeypatch.setattr(client_helper, "RESOURCE_ABSTRACTOR_ADDR", FAKE_ADDR)
    monkeypatch.setitem(client_helper._clients, FAKE_ADDR, fake_client)
    yield rec
