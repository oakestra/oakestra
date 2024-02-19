import ext_requests.mongodb_client as db
from bson import ObjectId

# ....... Gateway Operations ........ #
#######################################


def mongo_add_gateway_node(entry):
    db.app.logger.info("MONGODB - insert {} node to gateway nodes db...".format(entry["type"]))
    new_node = db.mongo_gateway_nodes.insert_one(entry)
    inserted_id = new_node.inserted_id
    db.app.logger.info("MONGODB - node {} added to gateway nodes db".format(str(inserted_id)))
    return str(inserted_id)


def mongo_delete_gateway_node(node_id):
    db.app.logger.info("MONGODB - delete node from gateway nodes db...")
    db.mongo_gateway_nodes.delete(ObjectId(node_id))
    db.app.logger.info("MONGODB - firewall {} deleted from gateway nodes db".format(str(node_id)))
    return


def mongo_get_node_by_id(node_id):
    return db.mongo_gateway_nodes.find_one(ObjectId(node_id))


def mongo_get_node_by_ip(node_ip):
    return db.mongo_gateway_nodes.find_one({"node_ip": node_ip})


def mongo_update_node_services(node_id, service_id, port):
    return db.mongo_gateway_nodes.update(
        ObjectId(node_id), {"$push": {"microservices": service_id, "used_ports": port}}
    )


def mongo_find_gateways_by_cluster(cluster_id):
    # retrieve all gateway nodes of cluster
    return db.mongo_gateway_nodes.find({"cluster_id": cluster_id})


# TODO
def mongo_update_gateway_status(cluster_id, status, status_note):
    db.mongo_gateway_nodes.insert_one(
        {"cluster_id": cluster_id, "status": status, "status_note": status_note}
    )


def mongo_delete_gateway(gateway_id):
    db.app.logger.info("MONGODB - delete gateway...")
    db.mongo_gateway_nodes.find_one_and_delete({"_id": ObjectId(gateway_id)})
    db.app.logger.info("MONGODB - gateway {} deleted")


def mongo_find_gateway_by_id(gateway_id):
    return db.mongo_gateway_nodes.find_one(ObjectId(gateway_id))


def mongo_get_all_gateways():
    return db.mongo_gateway_nodes.find()


# services #
# ........ #


def mongo_add_gateway_service(microservice):
    # add a service to be exposed to the database
    db.app.logger.info("MONGODB - insert service to gateway db...")
    new_service = db.mongo_gateway_services.insert_one(microservice)
    inserted_id = new_service.inserted_id
    db.app.logger.info("MONGODB - service {} added to gateway db".format(str(inserted_id)))
    return str(inserted_id)


def mongo_remove_gateway_service(microservice):
    # remove a service that was exposed from the database
    db.app.logger.info("MONGODB - remove service from gateway db...")
    db.mongo_gateway_services.find_one_and_delete(
        {"microserviceID": microservice["microserviceID"]}
    )
    db.app.logger.info("MONGODB - service removed from gateway db.")
    return


def mongo_get_gateway_service_by_id(microservice_id):
    # get information on exposed service
    return db.mongo_gateway_services.find_one({"microserviceID": microservice_id})


def mongo_get_clusters_of_active_service_instances(service_id):
    # get all cluster_ids of clusters that have running instances of the service to be exposed
    return db.mongo_services.distinct(
        "instance_list.cluster_id",
        {"instance_list.status": "RUNNING", "microserviceID": service_id},
    )


def mongo_get_service_instances_by_id(service_id):
    # get all deployed instances of a service, with information on the workers they are deployed on
    return db.mongo_services.find_one(
        {"microserviceID": service_id},
        {
            "instance_list.publicip": 1,
            "instance_list.cluster_id": 1,
            "port": 1,
            "job_name": 1,
            "instance_list.instance_number": 1,
        },
    )
