import utils.sla_validation as sla_validation


def test_full_sla():
    sla = {
        "app_name": "app",
        "app_ns": "app",
        "microservice_name": "service",
        "microservice_namespace": "service",
    }

    assert sla_validation.valid_sla(sla)

    sla["app_name"] = "app-"
    assert not sla_validation.valid_sla(sla)

    sla["app_name"] = "app"
    sla["app_ns"] = "app_"
    assert not sla_validation.valid_sla(sla)

    sla["app_ns"] = "app"
    sla["microservice_name"] = "service!"
    assert not sla_validation.valid_sla(sla)

    sla["microservice_name"] = "service"
    sla["microservice_namespace"] = "service."
    assert not sla_validation.valid_sla(sla)

    sla["microservice_name"] = "service1234safasdf"
    sla["microservice_namespace"] = "servicfdfwefd"
    assert sla_validation.valid_sla(sla)


def test_sla_missing_field():
    sla = {"app_name": "app", "app_ns": "app", "microservice_name": "service"}

    assert not sla_validation.valid_sla(sla)

    sla = {"app_name": "app", "app_ns": "app", "microservice_namespace": "service"}

    assert not sla_validation.valid_sla(sla)

    sla = {
        "app_name": "app",
        "microservice_name": "service",
        "microservice_namespace": "service",
    }

    assert not sla_validation.valid_sla(sla)

    sla = {
        "app_ns": "app",
        "microservice_name": "service",
        "microservice_namespace": "service",
    }

    assert not sla_validation.valid_sla(sla)
