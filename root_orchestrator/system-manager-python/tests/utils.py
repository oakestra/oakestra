from random import randint


def get_fake_sla_app(name, services=None):
    microservice_list = []
    if services is not None:
        for service in services:
            microservice_list.append(service)
    return {
        "sla_version": "v2.0",
        "customerID": "Admin",
        "applications": [
            {
                "application_name": name,
                "application_namespace": name,
                "application_desc": "No description here",
                "applicationID": "",
                "microservices": microservice_list,
            }
        ],
    }


def get_fake_db_app(name, services=None):
    microservice_list = []
    if services is not None:
        for service in services:
            microservice_list.append(service["microserviceID"])
    return {
        "application_name": name,
        "application_namespace": name,
        "applicationID": str(randint(0, 99999999)),
        "microservices": microservice_list,
    }


def get_fake_sla_service(
    name,
    namespace="test",
    image="",
    virt="container",
    requirements=None,
    addresses=None,
):
    if addresses is None:
        addresses = []
    return {
        "microserviceID": "",
        "microservice_name": name,
        "microservice_namespace": namespace,
        "virtualization": virt,
        "cmd": ["ls", "-al"],
        "memory": 0,
        "vcpus": 2,
        "vgpus": 0,
        "vtpus": 0,
        "bandwidth_in": 0,
        "bandwidth_out": 0,
        "storage": 0,
        "code": image,
        "state": "https://path.to.root.orchestrator.customer.application/example/state_1.json",
        "port": "80",
        "addresses": addresses,
    }


def get_fake_db_service(
    name,
    namespace="test",
    appname="app",
    appns="test",
    appid="42",
    image="",
    virt="docker",
    requirements=None,
    instances=None,
    rrip=None,
):
    instance_list = []
    if instances is not None:
        for instance in instances:
            instance_list.append(instance)

    return {
        "applicationID": appid,
        "microserviceID": str(randint(0, 9999999)),
        "app_name": appname,
        "app_ns": appns,
        "service_name": name,
        "service_ns": namespace,
        "image": image,
        "virtualization": virt,
        "next_instance_progressive_number": len(instance_list),
        "instance_list": instance_list,
        "RR_ip": rrip,
    }


def get_full_random_sla_app():
    app = get_fake_sla_app(str(randint(0, 99999)))
    for i in range(randint(1, 15)):
        service = get_fake_sla_service(
            str(i),
            image="docker.io/library/nginx:latest",
            virt="container",
            addresses={"RR_ip": "10.30.0." + str(i)},
        )
        app["applications"][0]["microservices"].append(service)
    return app
