import ext_requests.mongodb_client as db

# ....... Gateway Operations ........ #
#######################################

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
