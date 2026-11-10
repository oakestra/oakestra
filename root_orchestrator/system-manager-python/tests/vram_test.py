import pytest

# conftest stubs ext_requests.net_plugin_requests before service_management is imported
from services.service_management import generate_db_structure
from sla.v2_validator import validate_json_v2

from tests.utils import (
    get_fake_sla_app,
    get_fake_sla_service,
    get_first_app,
    get_full_random_sla_app,
)


def _sla_with(**resources):
    sla = get_full_random_sla_app()
    service = get_first_app(sla)["microservices"][0]
    service.update(resources)
    return sla


def test_schema_accepts_vram():
    assert validate_json_v2(_sla_with(vgpus=2, vram=16000.5)) is None


def test_schema_rejects_negative_vram():
    assert validate_json_v2(_sla_with(vram=-1)) is not None


def test_schema_rejects_negative_vgpus():
    assert validate_json_v2(_sla_with(vgpus=-1)) is not None


def _structure(virt="container", **resources):
    service = get_fake_sla_service("s", virt=virt, addresses={})
    service.pop("vgpus")
    service.update(resources)
    application = {**get_first_app(get_fake_sla_app("n")), "applicationID": "a"}
    return generate_db_structure(application, service)


@pytest.mark.parametrize("extra", [{}, {"vgpus": 0}])
def test_vram_without_vgpus_requests_one_gpu(extra):
    assert _structure(vram=8000, **extra)["vgpus"] == 1


def test_explicit_vgpus_kept_with_vram():
    assert _structure(vram=8000, vgpus=3)["vgpus"] == 3


@pytest.mark.parametrize("virt, extra", [("crosvm", {}), ("unikernel", {"arch": ["amd64"]})])
def test_vram_does_not_add_gpu_for_non_containers(virt, extra):
    assert "vgpus" not in _structure(virt=virt, vram=8000, **extra)


def test_no_vram_leaves_vgpus_untouched():
    assert "vgpus" not in _structure()
