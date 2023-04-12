import os
from flask_pymongo import PyMongo
from bson.objectid import ObjectId
from datetime import datetime

MONGO_URL = os.environ.get('CLOUD_MONGO_URL')
MONGO_PORT = os.environ.get('CLOUD_MONGO_PORT')

MONGO_ADDR_JOBS = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/jobs'
MONGO_ADDR_NET = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/netcache'
MONGO_ADDR_CLUSTER = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/cluster'

mongo_jobs = None
mongo_clusters = None
mongo_net = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 45


def mongo_init(flask_app):
    global app
    global mongo_jobs, mongo_net, mongo_clusters

    app = flask_app

    app.logger.info("Connecting to mongo...")

    # app.config["MONGO_URI"] = MONGO_ADDR
    try:
        mongo_jobs = PyMongo(app, uri=MONGO_ADDR_JOBS)
        mongo_net = PyMongo(app, uri=MONGO_ADDR_NET)
        mongo_clusters = PyMongo(app, uri=MONGO_ADDR_CLUSTER)
    except Exception as e:
        app.logger.fatal(e)
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
    job_content = {
        'system_job_id': obj.get('system_job_id'),
        'job_name': job_name,
        'service_ip_list': obj.get('service_ip_list'),
        'instance_list': [],
        **deployment_descriptor  # The content of the input deployment descriptor
    }
    if "_id" in job_content:
        del job_content['_id']
    # job insertion
    new_job = mongo_jobs.db.jobs.find_one_and_update(
        {'job_name': job_name},
        {'$set': job_content},
        upsert=True,
        return_document=True
    )
    app.logger.info("MONGODB - job {} inserted".format(str(new_job.get('_id'))))
    return str(new_job.get('_id'))


def mongo_remove_job(system_job_id):
    global mongo_jobs
    return mongo_jobs.db.jobs.delete_one({"system_job_id": system_job_id})


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
    for instance in instances:
        mongo_update_job_instance(job_id, instance)

    return mongo_jobs.db.jobs.find_one({'system_job_id': job_id})


def mongo_find_job_by_id(job_id):
    global mongo_jobs
    return mongo_jobs.db.jobs.find_one(ObjectId(job_id))


def mongo_find_job_by_systemid(sys_id):
    global mongo_jobs
    return mongo_jobs.db.jobs.find_one({"system_job_id": sys_id})


def mongo_find_job_by_name(job_name):
    global mongo_jobs
    return mongo_jobs.db.jobs.find_one({'job_name': job_name})

# TODO maybe explode IPv6? Check format consistency across all requests
def mongo_find_job_by_ip(ip):
    global mongo_jobs
    print("Got search job by IP request with IP:", ip)
    # Search by Service Ip
    job = mongo_jobs.db.jobs.find_one({'service_ip_list.Address': ip})
    if job is None:
        # Search by Service IPv6
        job = mongo_jobs.db.jobs.find_one({'service_ip_list.Address_v6': ip})
    if job is None:
        # Search by Instance ip
        job = mongo_jobs.db.jobs.find_one({'instance_list.instance_ip': ip})
    if job is None:
        # Search by Instance IPv6
        job = mongo_jobs.db.jobs.find_one({'instance_list.instance_ip_v6': ip})
    return job

"""
# TODO will need to be reworked, depending on architectural design decisions
def mongo_find_job_by_ip_v6(ip):
    global mongo_jobs
    # Search by Service IPv6
    job = mongo_jobs.db.jobs.find_one({'service_ip_list.Address_v6': ip})
    if job is None:
        # Search by instance IPv6
        job = mongo_jobs.db.jobs.find_one({'instance_list.instance_ip_v6': ip})
    return job
"""

def mongo_update_job_instance(system_job_id, instance):
    global mongo_jobs
    print('Updating job instance')
    mongo_jobs.db.jobs.update_one(
        {
            'system_job_id': system_job_id,
            "instance_list": {'$elemMatch': {'instance_number': instance['instance_number']}}},
        {
            '$set': {
                "instance_list.$.namespace_ip": instance.get('namespace_ip'),
                "instance_list.$.namespace_ip_v6": instance.get('namespace_ip_v6'),
                "instance_list.$.host_ip": instance.get('host_ip'),
                "instance_list.$.host_port": instance.get('host_port'),
            }
        }
    )


def mongo_create_job_instance(system_job_id, instance):
    global mongo_jobs
    print('Updating job instance')
    if not mongo_jobs.db.jobs.find_one(
            {
                "system_job_id": system_job_id,
                "instance_list.instance_number": instance["instance_number"]
            }):
        mongo_jobs.db.jobs.update_one(
            {'system_job_id': system_job_id},
            {
                '$push': {
                    "instance_list": instance
                }
            }
        )
    else:
        mongo_update_job_instance(system_job_id, instance)


def mongo_update_clean_one_instance(system_job_id, instance_number):
    """
    returns the replicas left
    """
    global mongo_jobs
    if instance_number == -1:
        return mongo_jobs.db.jobs.update_one({'system_job_id': system_job_id},
                                             {'$set': {'instance_list': []}})
    else:
        return mongo_jobs.db.jobs.update_one({'system_job_id': system_job_id},
                                             {'$pull': {'instance_list': {'instance_number': instance_number}}})


# ........... SERVICE MANAGER OPERATIONS  ............
######################################################

def mongo_get_service_address_from_cache():
    """
    Pop an available Service address, if any, from the free addresses cache
    @return: int[4] in the shape [10,30,x,y]
    """
    global mongo_net
    netdb = mongo_net.db.netcache

    entry = netdb.find_one({'type': 'free_service_ip'})

    if entry is not None:
        netdb.delete_one({"_id": entry["_id"]})
        return entry["ipv4"]
    else:
        return None


def mongo_free_service_address_to_cache(address):
    """
    Add back an address to the cache
    @param address: int[4] in the shape [10,30,x,y]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    assert len(address) == 4
    for n in address:
        assert 0 <= n < 254

    netcache.insert_one({
        'type': 'free_service_ip',
        'ipv4': address
    })


def mongo_get_next_service_ip():
    """
    Returns the next available ip address from the addressing space 10.30.x.y/16
    @return: int[4] in the shape [10,30,x,y,]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    next_addr = netcache.find_one({'type': 'next_service_ip'})

    if next_addr is not None:
        return next_addr["ipv4"]
    else:
        ip4arr = [10, 30, 0, 0]
        netcache = mongo_net.db.netcache
        id = netcache.insert_one({
            'type': 'next_service_ip',
            'ipv4': ip4arr
        })
        return ip4arr


def mongo_update_next_service_ip(address):
    """
    Update the value for the next service ip available
    @param address: int[4] in the form [10,30,x,y] monotonically increasing with respect to the previous address
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    # sanity check for the address
    assert len(address) == 4
    for n in address:
        assert 0 <= n < 256
    assert address[0] == 10
    assert address[1] == 30

    netcache.update_one({'type': 'next_service_ip'}, {'$set': {'ipv4': address}})


def mongo_get_next_subnet_ip():
    """
    Returns the next available subnetwork ip address from the addressing space 10.16.y.z/12
    @return: int[4] in the shape [10,x,y,z]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    next_addr = netcache.find_one({'type': 'next_subnet_ip'})

    if next_addr is not None:
        return next_addr["ipv4"]
    else:
        ip4arr = [10, 18, 0, 0]
        netcache = mongo_net.db.netcache
        id = netcache.insert_one({
            'type': 'next_subnet_ip',
            'ipv4': ip4arr
        })
        return ip4arr


def mongo_update_next_subnet_ip(address):
    """
    Update the value for the next subnet ip available
    @param address: int[4] in the form [10,x,y,z] monotonically increasing with respect to the previous address
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    # sanity check for the address
    assert len(address) == 4
    for n in address:
        assert 0 <= n < 256
    assert address[0] == 10
    assert 17 < address[1] < 30

    netcache.update_one({'type': 'next_subnet_ip'}, {'$set': {'ipv4': address}})


def mongo_get_subnet_address_from_cache():
    """
    Pop an available Subnet address, if any, from the free addresses cache
    @return: int[4] in the shape [10,x,y,z]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    entry = netcache.find_one({'type': 'free_subnet_ip'})

    if entry is not None:
        netcache.delete_one({"_id": entry["_id"]})
        return entry["ipv4"]
    else:
        return None


def mongo_free_subnet_address_to_cache(address):
    """
    Add back a subnetwork address to the cache
    @param address: int[4] in the shape [10,30,x,y]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    assert len(address) == 4
    for n in address:
        assert 0 <= n < 256

    netcache.insert_one({
        'type': 'free_subnet_ip',
        'ipv4': address
    })

# ........... IPv6 ................................#
####################################################

def mongo_get_service_address_from_cache_v6():
    """
    Pop an available Service address, if any, from the free addresses cache
    @return: int[16] in the shape [253, 255, [0, 8], a, b, c, d, e, f, g, h, i, j, k, l, m]
             equal to [fdff:[00, 08]00::]
    """
    global mongo_net
    netdb = mongo_net.db.netcache

    entry = netdb.find_one({'type': 'free_service_ipv6'})

    if entry is not None:
        netdb.delete_one({"_id": entry["_id"]})
        return entry["ipv6"]
    else:
        return None


def mongo_free_service_address_to_cache_v6(address):
    """
    Add back an address to the cache
    @param address: int[16] in the shape [253, 255, [0, 8], a, b, c, d, e, f, g, h, i, j, k, l, m]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    assert len(address) == 16
    for n in address:
        assert 0 <= n < 256
    
    assert address[0] == 253
    assert address[1] == 255
    assert address[2] == 0 or address[2] == 8

    netcache.insert_one({
        'type': 'free_service_ipv6',
        'ipv6': address
    })


def mongo_get_next_service_ip_v6():
    """
    Returns the next available ip address from the addressing space fdff:ffff:ffff:ffff::/64
    @return: int[16] in the shape [253, 255, [0, 8], a, b, c, d, e, f, g, h, i, j, k, l, m]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    next_addr = netcache.find_one({'type': 'next_service_ipv6'})

    if next_addr is not None:
        return next_addr["ipv6"]
    else:
        ipv6arr = [253, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
        netcache = mongo_net.db.netcache
        id = netcache.insert_one({
            'type': 'next_service_ipv6',
            'ipv6': ipv6arr
        })
        return ipv6arr


def mongo_update_next_service_ip_v6(address):
    """
    Update the value for the next service ip available
    @param address: int[16] in the form [253, 255, 0, a, b, c, d, e, f, g, h, i, j, k, l, m] 
        monotonically increasing with respect to the previous address
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    # sanity check for the address
    assert len(address) == 16
    for n in address:
        assert 0 <= n < 256

    assert address[0] == 253
    assert address[1] == 255
    assert address[2] == 0 or address[2] == 8

    netcache.update_one({'type': 'next_service_ipv6'}, {'$set': {'ipv6': address}})


def mongo_get_next_subnet_ip_v6():
    """
    Returns the next available subnetwork ip address from the addressing space fc00::/7
    @return: int[16] in the shape [25[2-3], a, b, c, d, e, f, g, h, i, j, k, l, m, n, 0] 
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    next_addr = netcache.find_one({'type': 'next_subnet_ipv6'})

    if next_addr is not None:
        return next_addr["ipv6"]
    else:
        ipv6arr = [252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
        netcache = mongo_net.db.netcache
        id = netcache.insert_one({
            'type': 'next_subnet_ipv6',
            'ipv6': ipv6arr
        })
        return ipv6arr


def mongo_update_next_subnet_ip_v6(address):
    """
    Update the value for the next subnet ip available
    @param address: int[16] in the form [252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    monotonically increasing with respect to the previous address
    """
    global mongo_net
    netcache = mongo_net.db.netcache
    
    # sanity check for the address
    assert len(address) == 16
    for n in address:
        assert 0 <= n < 256
    assert 252 <= address[0] <= 253
    netcache.update_one({'type': 'next_subnet_ipv6'}, {'$set': {'ipv6': address}})


def mongo_get_subnet_address_from_cache_v6():
    """
    Pop an available Subnet address, if any, from the free addresses cache
    @return: int[16] in the shape [252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0 ,0]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    entry = netcache.find_one({'type': 'free_subnet_ipv6'})

    if entry is not None:
        netcache.delete_one({"_id": entry["_id"]})
        return entry["ipv6"]
    else:
        return None


def mongo_free_subnet_address_to_cache_v6(address):
    """
    Add back a subnetwork address to the cache
    @param address: int[16] in the shape [252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0 ,0]
    """
    global mongo_net
    netcache = mongo_net.db.netcache

    assert len(address) == 16
    for n in address:
        assert 0 <= n < 256

    netcache.insert_one({
        'type': 'free_subnet_ipv6',
        'ipv6': address
    })

# ......... CLUSTER OPERATIONS ....................#
####################################################

def mongo_cluster_add(cluster_id, cluster_port, cluster_address, status):
    global mongo_clusters

    mongo_clusters.db.cluster.find_one_and_update(
        {"cluster_id": cluster_id},
        {'$set':
            {
                "cluster_port": cluster_port,
                "cluster_address": cluster_address,
                "status": status,
                "cluster_id": cluster_id
            }
        }, upsert=True)


def mongo_set_cluster_status(cluster_id, cluster_status):
    global mongo_clusters

    job = mongo_clusters.db.cluster.find_one_and_update(
        {"cluster_id": cluster_id},
        {'$set':
             {"status": cluster_status}
         })


def mongo_cluster_remove(cluster_id):
    global mongo_clusters
    mongo_clusters.db.cluster.delete_one({"cluster_id": cluster_id})


def mongo_get_cluster_by_ip(cluster_ip):
    global mongo_clusters
    return mongo_clusters.db.cluster.find_one({"cluster_address": cluster_ip})


# .......... INTERESTS OPERATIONS .........#
###########################################

def mongo_get_cluster_interested_to_job(job_name):
    global mongo_clusters
    return mongo_clusters.db.cluster.find({"interests": job_name})


def mongo_register_cluster_job_interest(cluster_id, job_name):
    global mongo_clusters
    interests = mongo_clusters.db.cluster.find_one({"cluster_id": cluster_id}).get("interests")
    if interests is None:
        interests = []
    if job_name in interests:
        return
    interests.append(job_name)
    mongo_clusters.db.cluster.find_one_and_update(
        {"cluster_id": cluster_id},
        {'$set': {
            "interests": interests
        }}
    )


def mongo_remove_cluster_job_interest(cluster_id, job_name):
    global mongo_clusters
    interests = mongo_clusters.db.cluster.find_one({"cluster_id": cluster_id}).get("interests")
    if interests is not None:
        if job_name in interests:
            interests.remove(job_name)
            mongo_clusters.db.cluster.find_one_and_update(
                {"cluster_id": cluster_id},
                {'$set': {
                    "interests": interests
                }}
    )
