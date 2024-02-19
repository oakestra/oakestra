import logging
from re import search

from ext_requests.cluster_requests import cluster_request_to_deploy_gateway
from ext_requests.gateway_db import (
    mongo_add_gateway_service,
    mongo_get_clusters_of_active_service_instances,
    mongo_get_gateway_service_by_id,
    mongo_get_service_instances_by_id,
    mongo_remove_gateway_service,
)
from sla.versioned_sla_parser import parse_sla_json


def create_gateway_service(current_user, sla):
    # TODO: implement current_user check of service, maybe use organizations?
    data = parse_sla_json(sla)
    logging.log(logging.INFO, sla)
    services = data.get("microservices")

    for microservice in services:
        # check if service is deployed
        service = mongo_get_service_instances_by_id(microservice["microserviceID"])
        if service is None:
            return {"message": "service {} not found".format(microservice["microserviceID"])}, 500

        # check if service is already exposed
        duplicate = mongo_get_gateway_service_by_id(microservice["microserviceID"])
        if duplicate is not None:
            return {
                "message": "service {} already exposed".format(microservice["microserviceID"])
            }, 500

        # fetch the internal port correctly
        # here we try to extract the non-docker-internal port on the target machine
        try:
            port = search(":(.+)", service["port"]).group(1)
        except AttributeError:
            port = service["port"]
        # remove protocol at the end, if present
        microservice["internal_port"] = int(port.split("/")[0])  # internal port
        microservice["job_name"] = service["job_name"]

        # add the service to be exposed to collection of exposed services
        mongo_add_gateway_service(microservice)
        microservice["_id"] = str(microservice["_id"])

        # fetch the clusters of running target service instances
        clusters = mongo_get_clusters_of_active_service_instances(microservice["microserviceID"])
        for cluster_id in clusters:
            # notify cluster to enable gateway for microservice if possible
            logging.log(
                logging.INFO,
                "deploying service {} on cluster {}".format(microservice, cluster_id),
            )
            gateways, status = cluster_request_to_deploy_gateway(cluster_id, microservice)
            # TODO: implement db operation
            print(gateways, status)
            if status != 200:
                mongo_remove_gateway_service(microservice)
                return {
                    "message": "cluster {} could not deploy gateway. Aborting.".format(cluster_id)
                }, 500

    # fetch gateways of service and add to return table
    # TODO: implement me
    # gateway_table[microservice['microserviceID']] = gateways
    return {"message": "services successfully exposed"}, 200


def get_service_gateway(user, service_id):
    return {"message" "implement me!"}, 200


def delete_service_gateway(user, service_id):
    return {"message" "implement me!"}, 200
