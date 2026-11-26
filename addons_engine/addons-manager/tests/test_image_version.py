import pytest
from api.v1.ui_addons_api import image_version

DIGEST = "sha256:" + "0123456789abcdef" * 4


@pytest.mark.parametrize(
    ("image_ref", "expected"),
    [
        ("ghcr.io/x/ui:0.1.0", "0.1.0"),
        ("ui:1.2", "1.2"),
        ("localhost:5000/ui", "latest"),
        ("localhost:5000/ui:2.0", "2.0"),
        (f"ghcr.io/x/ui@{DIGEST}", "sha256:0123456789ab"),
        (f"ghcr.io/x/ui:1.4.0@{DIGEST}", "1.4.0"),
        (f"localhost:5000/ui@{DIGEST}", "sha256:0123456789ab"),
        ("ui", "latest"),
    ],
)
def test_image_version(image_ref, expected):
    assert image_version(image_ref) == expected
