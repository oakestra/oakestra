import mongodb_client as db
from bson import ObjectId


def mongo_add_gateway_node(gateway):
    # remove node from netmanager table
    mongo_delete_netmanager_client(gateway["gateway_id"])

    # add node to gateway table
    db.app.logger.info("MONGODB - insert gateway node to gateway db...")
    new_gateway = db.mongo_gateway_nodes.insert_one(gateway)
    inserted_id = new_gateway.inserted_id
    db.app.logger.info("MONGODB - node {} added to gateway db".format(str(inserted_id)))
    return str(inserted_id)


def mongo_add_gateway_service_to_node(gateway_id, service):
    db.mongo_gateway_nodes.update_one(
        {"gateway_id": gateway_id},
        {"$addToSet": {"services": service, "used_ports": service["exposed_port"]}},
    )


def mongo_get_gateway_node(node_id):
    return db.mongo_gateway_nodes.find_one({"worker_id": node_id})


def mongo_delete_gateway_node(gateway_id):
    return db.mongo_gateway_nodes.delete_one({"gateway_id": gateway_id})


def mongo_find_available_gateway_by_port(port):
    return db.mongo_gateway_nodes.find_one({"used_ports": {"$nin": [port]}})


def mongo_get_gateways_of_service(service_id):
    return db.mongo_gateway_nodes.find({"microservices": service_id})


def mongo_get_service_instance_node_information(service_id):
    return db.mongo_jobs.db.jobs.distinct("instance_list.worker_id", {"microserviceID": service_id})


# ............................... #
#   Gateway Service operations    #
# ............................... #


def mongo_add_gateway_service(service):
    db.app.logger.info("MONGODB - insert service to gateway db...")
    new_service = db.mongo_gateway_services.insert_one(service)
    inserted_id = new_service.inserted_id
    db.app.logger.info("MONGODB - service {} added to gateway db".format(str(inserted_id)))


def mongo_get_gateway_service(service_id):
    db.app.logger.info(
        "MONGODB - looking for a gateway already exposing microservice {}".format(service_id)
    )
    return db.mongo_gateway_services.find_one({"microserviceID": service_id})


def mongo_delete_gateway_service(service_id):
    db.app.logger.info("MONGODB - delete service from gateway db...")
    db.mongo_gateway_services.find_one_and_delete({"microserviceID": service_id})
    db.app.logger.info("MONGODB - service {} deleted from gateway db".format(str(service_id)))
    return


def mongo_get_gateway_service_by_exposed_port(port_num):
    return db.mongo_gateway_services.find_one({"exposed_port": port_num})


def mongo_check_if_port_already_used(port_num):
    return db.mongo_gateway_services.find_one({"exposed_port": port_num}).limit(1).size()


# ........................ #
#   NetManager operations  #
# ........................ #


def mongo_register_netmanager_client(node_data):
    # add new idle node to database
    db.app.logger.info("MONGODB - insert new netmanager to gateway netmanagers db...")
    new_manager = db.mongo_gateway_netmanagers.insert_one(node_data)
    inserted_id = new_manager.inserted_id
    db.app.logger.info("MONGODB - node {} added to gateway netmanagers db".format(str(inserted_id)))
    return str(inserted_id)


def mongo_delete_netmanager_client(netmanager_id):
    # delete idle node from database
    db.app.logger.info("MONGODB - delete netmanager from gateway netmanager db...")
    db.mongo_gateway_netmanagers.find_one_and_delete({"_id": ObjectId(netmanager_id)})
    db.app.logger.info(
        "MONGODB - deleted netmanager client {} from gateway netmanagers db".format(netmanager_id)
    )


def mongo_find_available_idle_worker():
    # fetch one idle node from database
    db.app.logger.info("MONGODB - fetching idle netmanager")
    return db.mongo_gateway_netmanagers.find_one()
