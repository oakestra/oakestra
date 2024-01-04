import os
from datetime import datetime

from bson.objectid import ObjectId
from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("CLUSTER_MONGO_URL")
MONGO_PORT = os.environ.get("CLUSTER_MONGO_PORT")

MONGO_ADDR_NODES = "mongodb://" + str(MONGO_URL) + ":" + str(MONGO_PORT) + "/nodes"
MONGO_ADDR_JOBS = "mongodb://" + str(MONGO_URL) + ":" + str(MONGO_PORT) + "/jobs"

mongo_nodes = None
mongo_jobs = None
app = None


def mongo_init(flask_app):
    global app
    global mongo_nodes, mongo_jobs

    app = flask_app

    mongo_nodes = PyMongo(app, uri=MONGO_ADDR_NODES)
    mongo_jobs = PyMongo(app, uri=MONGO_ADDR_JOBS)

    app.logger.info("MONGODB - init mongo")


# ................. Worker Node Operations ...............#
###########################################################


def mongo_upsert_node(obj):
    global app, mongo_nodes
    app.logger.info("MONGODB - upserting node...")
    json_node_info = obj["node_info"]
    node_info_hostname = json_node_info.get("host")

    nodes = mongo_nodes.db.nodes
    # find node by hostname and if it exists, just upsert
    node_id = nodes.find_one_and_update(
        {"node_info.host": node_info_hostname},
        {
            "$set": {
                "node_info": json_node_info,
                "node_address": obj.get("ip"),
                "node_subnet": obj.get("node_subnet"),
            }
        },
        upsert=True,
        return_document=True,
    ).get("_id")
    app.logger.info(node_id)
    return node_id


def mongo_find_node_by_id(node_id):
    global mongo_nodes
    return mongo_nodes.db.nodes.find_one(node_id)


def mongo_find_node_by_name(node_name):
    global mongo_nodes
    try:
        return mongo_nodes.db.nodes.find_one({"node_info.host": node_name})
    except Exception:
        return "Error"


def mongo_find_node_by_id_and_update_cpu_mem(node_id, node_payload):
    global app, mongo_nodes
    app.logger.info("MONGODB - update cpu and memory of worker node {0} ...".format(node_id))
    # o = mongo.db.nodes.find_one({'_id': node_id})
    # print(o)

    time_now = datetime.now()

    mongo_nodes.db.nodes.find_one_and_update(
        {"_id": ObjectId(node_id)},
        {
            "$set": {
                "current_cpu_percent": node_payload.get("cpu", 0),
                "current_cpu_cores_free": node_payload.get("free_cores", 0),
                "current_memory_percent": node_payload.get("memory", 0),
                "current_free_memory_in_MB": node_payload.get("memory_free_in_MB", 0),
                "gpu_driver": node_payload.get("gpu_driver", "-"),
                "gpu_usage": node_payload.get("gpu_usage", 0),
                "gpu_cores": node_payload.get("gpu_cores", 0),
                "gpu_temp": node_payload.get("gpu_temp", 0),
                "gpu_mem_used": node_payload.get("gpu_mem_used", 0),
                "gpu_tot_mem": node_payload.get("gpu_tot_mem", 0),
                "last_modified": time_now,
                "last_modified_timestamp": datetime.timestamp(time_now),
            }
        },
        upsert=True,
    )

    return 1


def find_one_edge_node():
    """Find first occurrence of edge nodes"""
    global mongo_nodes
    return mongo_nodes.db.nodes.find_one()


def find_all_nodes():
    global mongo_nodes
    return mongo_nodes.db.nodes.find()


def mongo_dead_nodes():
    print("looking for dead nodes")


def mongo_aggregate_node_information(TIME_INTERVAL):
    """1. Find all nodes"""
    """ 2. Aggregate cpu, memory, and more information of worker nodes"""

    global mongo_nodes

    cumulative_cpu = 0
    cumulative_cpu_cores = 0
    cumulative_memory = 0
    gpu_tot_mem = 0
    gpu_mem_used = 0
    gpu_temp = 0
    gpu_drivers = []
    gpu_usage = 0
    gpu_cores = 0
    cumulative_memory_in_mb = 0
    number_of_active_nodes = 0
    technology = []
    aggregation_per_architecture = {}

    nodes = find_all_nodes()
    for n in nodes:
        # print(n)

        # if it is not older than TIME_INTERVAL
        try:
            if n.get("last_modified_timestamp") >= (datetime.now().timestamp() - TIME_INTERVAL):
                cumulative_cpu += n.get("current_cpu_percent", 0)
                cumulative_cpu_cores += n.get("current_cpu_cores_free", 0)
                cumulative_memory += n.get("current_memory_percent", 0)
                cumulative_memory_in_mb += n.get("current_free_memory_in_MB", 0)
                gpu_drivers.append(n.get("gpu_driver", "-"))
                gpu_usage += n.get("gpu_usage", 0)
                gpu_cores += n.get("gpu_cores", 0)
                gpu_temp += n.get("gpu_temp", 0)
                gpu_tot_mem += n.get("gpu_tot_mem", 0)
                gpu_mem_used += n.get("gpu_mem_used", 0)
                number_of_active_nodes += 1
                for t in n.get("node_info").get("technology"):
                    technology.append(t) if t not in technology else technology

                arch = n.get("node_info").get("architecture")
                if arch is not None:
                    aggregation = None
                    if aggregation_per_architecture.get(arch, None) is None:
                        aggregation_per_architecture[arch] = {}
                        aggregation = aggregation_per_architecture[arch]
                        aggregation["cpu_percent"] = 0
                        aggregation["cpu_cores"] = 0
                        aggregation["memory"] = 0
                        aggregation["memory_in_mb"] = 0

                    aggregation = aggregation_per_architecture[arch]
                    aggregation["cpu_percent"] += n.get("current_cpu_percent", 0)
                    aggregation["cpu_cores"] += n.get("current_cpu_cores_free", 0)
                    aggregation["memory"] += n.get("current_memory_percent", 0)
                    aggregation["memory_in_mb"] += n.get("current_free_memory_in_MB", 0)
                    # GPU not aggregated for unikernel
            else:
                print("Node {0} is inactive.".format(n.get("_id")))
        except Exception as e:
            print(
                "Problem during the aggregation of the data, skipping the node: ",
                str(n),
                " - because - ",
                str(e),
            )

    mongo_update_jobs_status(TIME_INTERVAL)
    jobs = mongo_find_all_jobs()

    return {
        "cpu_percent": cumulative_cpu,
        "memory_percent": cumulative_memory,
        "cpu_cores": cumulative_cpu_cores,
        "cumulative_memory_in_mb": cumulative_memory_in_mb,
        "gpu_drivers": gpu_drivers,
        "gpu_percent": gpu_usage,
        "gpu_cores": gpu_cores,
        "gpu_temp": gpu_temp,
        "gpu_tot_mem": gpu_tot_mem,
        "gpu_mem_used": gpu_mem_used,
        "number_of_nodes": number_of_active_nodes,
        "jobs": jobs,
        "virtualization": technology,
        "aggregation_per_architecture": aggregation_per_architecture,
        "more": 0,
    }


# ................. Job Operations .......................#
###########################################################


def mongo_create_new_job_instance(job, system_job_id, instance_number):
    print("insert/upsert requested job")
    job["system_job_id"] = system_job_id
    del job["_id"]
    if job.get("instance_list") is not None:
        del job["instance_list"]
    result = mongo_jobs.db.jobs.find_one_and_update(
        {"system_job_id": str(job["system_job_id"])},
        {"$set": job},
        upsert=True,
        return_document=True,
    )  # if job does not exist, insert it
    if result.get("instance_list") is None:
        result["instance_list"] = []
    result["instance_list"].append(
        {"instance_number": instance_number, "status": "CLUSTER_SCHEDULED"}
    )
    mongo_jobs.db.jobs.find_one_and_update(
        {"system_job_id": str(job["system_job_id"])},
        {"$set": {"instance_list": result["instance_list"]}},
    )
    result["_id"] = str(result["_id"])
    return result


def mongo_find_job_by_system_id(system_job_id):
    return mongo_jobs.db.jobs.find_one({"system_job_id": str(system_job_id)})


def mongo_find_job_by_id(id):
    print("Find job by Id")
    return mongo_jobs.db.jobs.find_one({"_id": ObjectId(id)})


def mongo_update_jobs_status(TIME_INTERVAL):
    "If there are no updates from a job in the last TIME_INTERVAL mark it as failed"
    jobs = mongo_find_all_jobs()
    for job in jobs:
        try:
            updated = False
            status = "RUNNING"
            for instance in range(len(job["instance_list"])):
                if job["instance_list"][instance].get(
                    "last_modified_timestamp", datetime.now().timestamp()
                ) < (datetime.now().timestamp() - TIME_INTERVAL) and job["instance_list"][
                    instance
                ].get(
                    "status", 0
                ) not in [
                    "NODE_SCHEDULED",
                    "CLUSTER_SCHEDULED",
                ]:
                    print("Job is inactive: " + str(job.get("job_name")))
                    job["instance_list"][instance]["status"] = "FAILED"
                    status = "FAILED"
                    updated = True
            if updated:
                mongo_jobs.db.jobs.update_one(
                    {"system_job_id": str(job["system_job_id"])},
                    {"$set": {"instance_list": job["instance_list"], "status": status}},
                )
        except Exception as e:
            print(e)


def mongo_find_all_jobs():
    global mongo_jobs
    # list (= going into RAM) okey for small result sets (not clean for large data sets!)
    return list(
        mongo_jobs.db.jobs.find(
            {},
            {
                "_id": 0,
                "system_job_id": 1,
                "job_name": 1,
                "status": 1,
                "instance_list": 1,
            },
        )
    )


def mongo_find_job_by_name(job_name):
    global mongo_jobs
    return mongo_jobs.db.jobs.find_one({"job_name": job_name})


def mongo_find_job_by_ip(ip):
    global mongo_jobs
    # Search by Service Ip
    job = mongo_jobs.db.jobs.find_one({"service_ip_list.Address": ip})
    if job is None:
        # Search by instance ip
        job = mongo_jobs.db.jobs.find_one({"instance_list.instance_ip": ip})
    return job


def mongo_update_job_status(system_job_id, instancenum, status, node):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({"system_job_id": str(system_job_id)})
    instance_list = job["instance_list"]
    for instance in instance_list:
        if int(instance.get("instance_number")) == int(instancenum):
            instance["status"] = status
            if node is not None:
                instance["host_ip"] = node["node_address"]
                port = node["node_info"].get("node_port")
                if port is None:
                    port = 50011
                instance["host_port"] = port
                instance["worker_id"] = node.get("_id")
            break
    return mongo_jobs.db.jobs.update_one(
        {"system_job_id": str(system_job_id)},
        {"$set": {"status": status, "instance_list": instance_list}},
    )


def mongo_get_services_with_failed_instanes():
    return mongo_jobs.db.jobs.find(
        {
            "$or": [
                {"instance_list.status": "FAILED"},
                {"instance_list.status": "DEAD"},
                {"instance_list.status": "NO_WORKER_CAPACITY"},
            ]
        }
    )


def mongo_update_job_deployed(sname, instance_num, status, publicip, workerid):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({"job_name": sname})
    if job:
        instance_list = job.get("instance_list", [])
        updated = False
        for instance in range(len(instance_list)):
            if int(instance_list[instance]["instance_number"]) == int(instance_num):
                if instance_list[instance].get("worker_id") != workerid:
                    return None  # cannot update another worker's resources
                instance_list[instance]["status"] = status
                instance_list[instance]["publicip"] = publicip
                updated = True
        if updated:
            return mongo_jobs.db.jobs.update_one(
                {"job_name": sname}, {"$set": {"instance_list": instance_list}}
            )
    return None


def mongo_update_service_resources(sname, service, workerid, instance_num=0):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({"job_name": sname})
    if job:
        instance_list = job["instance_list"]
        for instance in range(len(instance_list)):
            if int(instance_list[instance]["instance_number"]) == int(instance_num):
                if instance_list[instance].get("worker_id") != workerid:
                    return None  # cannot update another worker's resources
                instance_list[instance]["status"] = "RUNNING"
                instance_list[instance]["status_detail"] = service.get("status_detail")
                instance_list[instance]["last_modified_timestamp"] = datetime.timestamp(
                    datetime.now()
                )
                instance_list[instance]["cpu"] = service.get("cpu")
                instance_list[instance]["memory"] = service.get("memory")
                instance_list[instance]["disk"] = service.get("disk")
                instance_list[instance]["logs"] = service.get("logs", "")
                return mongo_jobs.db.jobs.update_one(
                    {"job_name": sname}, {"$set": {"instance_list": instance_list}}
                )
    else:
        return None


def mongo_remove_job_instance(system_job_id, instance_number):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({"system_job_id": str(system_job_id)})
    instances = job["instance_list"]
    for instance in instances:
        if int(instance["instance_number"]) == int(instance_number) or int(instance_number) == -1:
            instances.remove(instance)
            break
    if len(instances) < 1:
        print("Removing job")
        print(job)
        return mongo_jobs.db.jobs.find_one_and_delete({"system_job_id": str(system_job_id)})
    else:
        return mongo_jobs.db.jobs.update_one(
            {"system_job_id": str(system_job_id)},
            {"$set": {"instance_list": instances}},
        )
