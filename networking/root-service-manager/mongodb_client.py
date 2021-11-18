import os
from flask_pymongo import PyMongo
from bson.objectid import ObjectId
from datetime import datetime

MONGO_URL = os.environ.get('CLOUD_MONGO_URL')
MONGO_PORT = os.environ.get('CLOUD_MONGO_PORT')

MONGO_ADDR_JOBS = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/jobs'
MONGO_ADDR_NET = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/netcache'

mongo_clusters = None
mongo_jobs = None
mongo_net = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 45


def mongo_init(flask_app):
    global app
    global mongo_clusters, mongo_jobs, mongo_net

    app = flask_app

    # app.config["MONGO_URI"] = MONGO_ADDR
    mongo_clusters = PyMongo(app, uri=MONGO_ADDR_CLUSTERS)
    mongo_jobs = PyMongo(app, uri=MONGO_ADDR_JOBS)
    mongo_net = PyMongo(app, uri=MONGO_ADDR_NET)
    app.logger.info("MONGODB - init mongo")


# ......... JOB OPERATIONS .........................
####################################################

def mongo_insert_job(obj):
    global mongo_jobs
    app.logger.info("MONGODB - insert job...")
    deployment_descriptor = obj['deployment_descriptor']

    # jobname and details generation
    job_name = deployment_descriptor['app_name'] \
               + "." + deployment_descriptor['app_ns'] \
               + "." + deployment_descriptor['service_name'] \
               + "." + deployment_descriptor['service_ns']
    file['job_name'] = job_name
    job_content = {
        'system_job_id': obj.get('system_job_id'),
        'job_name': job_name,
        'service_ip_list': obj.get('service_ip_list'),
        **deployment_descriptor  # The content of the input deployment descriptor
    }
    # job insertion
    jobs = mongo_jobs.db.jobs
    new_job = jobs.find_one_and_update(
        {'job_name': job_name},
        {'$set': job_content},
        upsert=True,
        return_document=True
    )
    app.logger.info("MONGODB - job {} inserted".format(str(new_job.get('_id'))))
    return str(new_job.get('_id'))


def mongo_get_all_jobs():
    global mongo_jobs
    return mongo_jobs.db.jobs.find()


def mongo_get_job_status(job_id):
    global mongo_jobs
    return mongo_jobs.db.jobs.find_one({'_id': ObjectId(job_id)}, {'status': 1})['status'] + '\n'


def mongo_update_job_status(job_id, status):
    global mongo_jobs
    return mongo_jobs.db.jobs.update_one({'_id': ObjectId(job_id)}, {'$set': {'status': status}})


def mongo_update_job_net_status(job_id, instances):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({'_id': ObjectId(job_id)})
    instance_list = job['instance_list']
    for instance in instances:
        instance_num = instance['instance_number']
        elem = instance_list[instance_num]
        elem['namespace_ip'] = instance['namespace_ip']
        elem['host_ip'] = instance['host_ip']
        elem['host_port'] = instance['host_port']
        instance_list[instance_num] = elem
    mongo_jobs.db.jobs.update_one({'_id': ObjectId(job_id)}, {'$set': {'instance_list': instance_list}})


def mongo_find_job_by_id(job_id):
    global mongo_jobs
    return mongo_jobs.db.jobs.find_one(ObjectId(job_id))


def mongo_find_job_by_name(job_name):
    global mongo_jobs
    return mongo_jobs.db.jobs.find_one({'job_name': job_name})


def mongo_find_job_by_ip(ip):
    global mongo_jobs
    # Search by Service Ip
    job = mongo_jobs.db.jobs.find_one({'service_ip_list.Address': ip})
    if job is None:
        # Search by instance ip
        job = mongo_jobs.db.jobs.find_one({'instance_list.instance_ip': ip})
    return job

def mongo_update_job_status_and_instances_by_system_job_id(system_job_id, status, replicas, instance_list):
    global mongo_jobs
    print('Updating Job Status and assigning a cluster for this job...')
    mongo_jobs.db.jobs.update_one({'system_job_id': system_job_id},
                                  {'$set': {'status': status, 'replicas': replicas, 'instance_list': instance_list}})


# .......... BOTH CLUSTER and JOB OPERATIONS .........
######################################################

def mongo_find_cluster_of_job(job_id):
    app.logger.info('Find job by Id and return cluster...')
    job_obj = mongo_jobs.db.jobs.find_one({'_id': ObjectId(job_id)},
                                          {'instance_list': 1})  # return just the assgined cluster of the job
    cluster_id = ObjectId(job_obj.get('instance_list')[0].get('cluster_id'))
    return mongo_find_cluster_by_id(cluster_id)


# ........... SERVICE MANAGER OPERATIONS  ............
######################################################

def mongo_get_service_address_from_cache():
    """
    Pop an available Service address, if any, from the free addresses cache
    @return: int[4] in the shape [172,30,x,y]
    """
    global mongo_net
    netdb = mongo_net.db.net

    entry = netdb.find_one({'type': 'free_service_ip'})

    if entry is not None:
        netdb.delete_one({"_id": entry["_id"]})
        return entry["ipv4"]
    else:
        return None


def mongo_free_service_address_to_cache(address):
    """
    Add back an address to the cache
    @param address: int[4] in the shape [172,30,x,y]
    """
    global mongo_net
    netdb = mongo_net.db.net

    assert len(address) == 4
    for n in address:
        assert 0 <= n < 254

    netdb.insert({
        'type': 'free_service_ip',
        'ipv4': address
    })


def mongo_get_next_service_ip():
    """
    Returns the next available ip address from the addressing space 172.30.x.y/16
    @return: int[4] in the shape [172,30,x,y,]
    """
    global mongo_net
    netdb = mongo_net.db.net

    next_addr = netdb.find_one({'type': 'next_service_ip'})

    if next_addr is not None:
        return next_addr["ipv4"]
    else:
        ip4arr = [172, 30, 0, 0]
        id = netdb.insert({
            'type': 'next_service_ip',
            'ipv4': ip4arr
        })
        return ip4arr


def mongo_update_next_service_ip(address):
    """
    Update the value for the next service ip available
    @param address: int[4] in the form [172,30,x,y] monotonically increasing with respect to the previous address
    """
    global mongo_net
    netdb = mongo_net.db.net

    # sanity check for the address
    assert len(address) == 4
    for n in address:
        assert 0 <= n < 256
    assert address[0] == 172
    assert address[1] == 30

    netdb.update_one({'type': 'next_service_ip'}, {'$set': {'ipv4': address}})


def mongo_get_next_subnet_ip():
    """
    Returns the next available subnetwork ip address from the addressing space 172.16.y.z/12
    @return: int[4] in the shape [172,x,y,z]
    """
    global mongo_net
    netdb = mongo_net.db.net

    next_addr = netdb.find_one({'type': 'next_subnet_ip'})

    if next_addr is not None:
        return next_addr["ipv4"]
    else:
        ip4arr = [172, 18, 0, 0]
        id = netdb.insert({
            'type': 'next_subnet_ip',
            'ipv4': ip4arr
        })
        return ip4arr


def mongo_update_next_subnet_ip(address):
    """
    Update the value for the next subnet ip available
    @param address: int[4] in the form [172,x,y,z] monotonically increasing with respect to the previous address
    """
    global mongo_net
    netdb = mongo_net.db.net

    # sanity check for the address
    assert len(address) == 4
    for n in address:
        assert 0 <= n < 256
    assert address[0] == 172
    assert 17 < address[1] < 30

    netdb.update_one({'type': 'next_subnet_ip'}, {'$set': {'ipv4': address}})


def mongo_get_subnet_address_from_cache():
    """
    Pop an available Subnet address, if any, from the free addresses cache
    @return: int[4] in the shape [172,x,y,z]
    """
    global mongo_net
    netdb = mongo_net.db.net

    entry = netdb.find_one({'type': 'free_subnet_ip'})

    if entry is not None:
        netdb.delete_one({"_id": entry["_id"]})
        return entry["ipv4"]
    else:
        return None


def mongo_free_subnet_address_to_cache(address):
    """
    Add back a subnetwork address to the cache
    @param address: int[4] in the shape [172,30,x,y]
    """
    global mongo_net
    netdb = mongo_net.db.net

    assert len(address) == 4
    for n in address:
        assert 0 <= n < 256

    netdb.insert({
        'type': 'free_subnet_ip',
        'ipv4': address
    })
