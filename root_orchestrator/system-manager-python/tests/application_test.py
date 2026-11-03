from unittest.mock import patch

from services.application_management import update_app


def test_update_application_without_microservices_field_keeps_existing_services():
    with patch("services.application_management.app_operations") as app_operations:
        update_app("app-id", "Admin", {"application_name": "renamed"})

    app_operations.update_app.assert_called_once_with(
        "app-id", "Admin", {"application_name": "renamed"}
    )


def test_update_application_forwards_only_updatable_fields():
    fields = {
        "application_name": "app",
        "application_namespace": "ns",
        "application_desc": "desc",
        "microservices": ["s1"],
        "userId": "someone-else",
    }
    with patch("services.application_management.app_operations") as app_operations:
        update_app("app-id", "Admin", fields)

    app_operations.update_app.assert_called_once_with(
        "app-id",
        "Admin",
        {
            "application_name": "app",
            "application_namespace": "ns",
            "application_desc": "desc",
            "microservices": ["s1"],
        },
    )
