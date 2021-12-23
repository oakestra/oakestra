import os
from flask_pymongo import PyMongo
from bson.objectid import ObjectId

MONGO_URL = os.environ.get('CLUSTER_MONGO_URL')
MONGO_PORT = os.environ.get('CLUSTER_MONGO_PORT')

MONGO_ADDR_NODES = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/nodes'
MONGO_ADDR_JOBS = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/jobs'

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

def mongo_find_node_by_id_and_update_subnetwork(node_id, addr):
    global app, mongo_nodes
    app.logger.info('MONGODB - update subnetwork of worker node {0} ...'.format(node_id))

    mongo_nodes.db.nodes.find_one_and_update(
        {'_id': ObjectId(node_id)},
        {'$set': {'node_subnet': addr}},
        upsert=True)

    return 1


# ........... Job Operations ............#
#########################################

def mongo_insert_job(job_name, job, sip_list, instances):
    global mongo_jobs
    app.logger.info("MONGODB - insert job...")
    job_content = {
        'system_job_id': job.get('system_job_id'),
        'job_name': job_name,
        'service_ip_list': sip_list,
        'instance_list': instances,
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


def mongo_update_job_deployed(job_name, status, ns_ip, node_id, instance_number, host_ip, host_port):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({'job_name': job_name})
    instance_list = job['instance_list']
    for instance in instance_list:
        if int(instance["instance_number"]) == int(instance_number):
            instance['worker_id'] = node_id
            instance['namespace_ip'] = ns_ip
            instance['host_ip'] = host_ip
            instance['host_port'] = int(host_port)
            break
    return mongo_jobs.db.jobs.update_one({'job_name': job_name},
                                         {'$set': {'status': status, 'instance_list': instance_list}})


def mongo_find_job_by_id(id):
    print('Find job by Id')
    return mongo_jobs.db.jobs.find_one({'_id': ObjectId(id)})


def mongo_update_job_status(job_id, status, node):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({'_id': ObjectId(job_id)})
    instance_list = job['instance_list']
    for instance in instance_list:
        if instance.get('host_ip') == '':
            instance['host_ip'] = node['node_address']
            port = node['node_info'].get('node_port')
            if port is None:
                port = 50011
            instance['host_port'] = int(port)
            instance['worker_id'] = node.get('_id')
            break
    return mongo_jobs.db.jobs.update_one({'_id': ObjectId(job_id)},
                                         {'$set': {'status': status, 'instance_list': instance_list}})
