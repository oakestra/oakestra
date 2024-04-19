import utils.sla_validation as sla_validation

def test_full_sla():
    sla = {
        "app_name": "app",
        "app_ns": "app",
        "microservice_name": "service",
        "microservice_namespace": "service"
    }

    assert sla_validation.valid_sla(sla) == True

    sla["app_name"] = "app-"
    assert sla_validation.valid_sla(sla) == False

    sla["app_name"] = "app"
    sla["app_ns"] = "app_"
    assert sla_validation.valid_sla(sla) == False

    sla["app_ns"] = "app"
    sla["microservice_name"] = "service!"
    assert sla_validation.valid_sla(sla) == False

    sla["microservice_name"] = "service"
    sla["microservice_namespace"] = "service."
    assert sla_validation.valid_sla(sla) == False

    sla["microservice_name"] = "service1234safasdf"
    sla["microservice_namespace"] = "servicfdfwefd"
    assert sla_validation.valid_sla(sla) == True

def test_sla_missing_field():
    sla = {
        "app_name": "app",
        "app_ns": "app",
        "microservice_name": "service"
    }

    assert sla_validation.valid_sla(sla) == False

    sla = {
        "app_name": "app",
        "app_ns": "app",
        "microservice_namespace": "service"
    }

    assert sla_validation.valid_sla(sla) == False

    sla = {
        "app_name": "app",
        "microservice_name": "service",
        "microservice_namespace": "service"
    }

    assert sla_validation.valid_sla(sla) == False

    sla = {
        "app_ns": "app",
        "microservice_name": "service",
        "microservice_namespace": "service"
    }

    assert sla_validation.valid_sla(sla) == False