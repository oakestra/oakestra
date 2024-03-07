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

    #initialize jobs db
    mongo_jobs.db.jobs.drop()

    app.logger.info("MONGODB - init mongo")


# ................. Worker Node Operations ...............#
###########################################################

def mongo_find_node_by_id_and_update_subnetwork(node_id, addr, addr_v6):
    global app, mongo_nodes
    app.logger.info('MONGODB - update subnetwork of worker node {0} ...'.format(node_id))

    mongo_nodes.db.nodes.find_one_and_update(
        {'_id': ObjectId(node_id)},
        {'$set': {
            'node_subnet': addr,
            'node_subnet_v6': addr_v6
            }},
        upsert=True)

    return 1


# ........... Job Operations ............#
#########################################

def mongo_insert_job(job):
    global mongo_jobs
    app.logger.info("MONGODB - insert job...")
    job_content = {
        'system_job_id': job['system_job_id'],
        'job_name': job['job_name'],
        'service_ip_list': job['service_ip_list']
    }
    # job insertion
    jobs = mongo_jobs.db.jobs
    new_job = jobs.find_one_and_update(
        {'job_name': job['job_name']},
        {'$set': job_content},
        upsert=True,
        return_document=True
    )
    # if new job add empty instance list
    if new_job.get('instance_list') is None:
        jobs.find_one_and_update(
            {'job_name': job['job_name']},
            {'$set': {'instance_list': []}}
        )
    app.logger.info("MONGODB - job {} inserted".format(str(new_job.get('_id'))))
    return str(new_job.get('_id'))


def mongo_remove_job(job_name):
    global mongo_jobs
    mongo_jobs.db.jobs.delete_one({"job_name": job_name})


def mongo_update_job(job):
    if job is None:
        return
    if job.get("job_name", "") == "":
        return

    current_job = mongo_jobs.db.jobs.find_one(
        {
            'job_name': job.get("job_name")
        })

    # If job exists, update the instances
    if current_job is not None:
        for instance in job.get('instance_list', []):
            mongo_update_job_instance(job_name=job.get("job_name"), instance=instance)
    # Otherwise, insert the job
    else:
        mongo_insert_job(job)


def mongo_update_job_instance(job_name, instance):
    # update if exist otherwise push a new instance
    if mongo_jobs.db.jobs.find_one(
            {
                'job_name': job_name,
                "instance_list.instance_number": instance['instance_number']
            }):
        mongo_jobs.db.jobs.update_one(
            {
                'job_name': job_name,
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
    else:
        mongo_jobs.db.jobs.update_one(
            {
                'job_name': job_name,
            },
            {
                '$push': {"instance_list": instance},
            }
        )


def mongo_remove_job_instance(job_name, instance_number):
    global mongo_jobs
    delete = False
    if int(instance_number) > -1:
        job = mongo_jobs.db.jobs.find_one_and_update(
            {'job_name': job_name},
            {'$pull': {'instance_list': {'instance_number': instance_number}}},
            return_document=True
        )
        if job is not None:
            if job['instance_list'] is None:
                delete = True
            if len(job['instance_list']) < 1:
                delete = True
    else:
        delete = True
    if delete:
        mongo_remove_job(job_name)


def mongo_find_job_by_name(job_name):
    global mongo_jobs
    return mongo_jobs.db.jobs.find_one({'job_name': job_name})


def mongo_find_job_by_ip(ip):
    global mongo_jobs
    # Search by Service IP
    job = mongo_jobs.db.jobs.find_one({'service_ip_list.Address': ip})
    if job is None:
        # Search by Service IPv6
        job = mongo_jobs.db.jobs.find_one({'service_ip_list.Address_v6': ip})
    if job is None:
        # Search by Instance IP
        job = mongo_jobs.db.jobs.find_one({'instance_list.instance_ip': ip})
    if job is None:
        # Search by Instance IPv6
        job = mongo_jobs.db.jobs.find_one({'instance_list.instance_ip_v6': ip})
    return job

def mongo_update_job_deployed(job_name, status, ns_ip, ns_ipv6, node_id, instance_number, host_ip, host_port):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({'job_name': job_name})
    if job is None:
        return None
    instance_list = job.get('instance_list',[])
    for instance in instance_list:
        if int(instance["instance_number"]) == int(instance_number):
            instance['worker_id'] = node_id
            instance['namespace_ip'] = ns_ip
            instance['namespace_ip_v6'] = ns_ipv6
            instance['host_ip'] = host_ip
            instance['host_port'] = int(host_port)
            break
    return mongo_jobs.db.jobs.find_one_and_update({'job_name': job_name},
                                         {'$set': {'status': status, 'instance_list': instance_list}},
                                         return_document=True)


def mongo_find_job_by_id(id):
    print('Find job by Id')
    return mongo_jobs.db.jobs.find_one({'_id': ObjectId(id)})


# ........ Interest Operations .........#
#########################################

def mongo_get_interest_workers(job_name):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({'job_name': job_name})
    if job is not None:
        interested_nodes = job.get("interested_nodes")
        if interested_nodes is not None:
            return interested_nodes
    return []


def mongo_add_interest(job_name, clientid):
    global mongo_jobs
    interested_nodes = mongo_get_interest_workers(job_name)
    interested_nodes.append(clientid)
    mongo_jobs.db.jobs.update_one(
        {'job_name': job_name},
        {'$set': {
            "interested_nodes": interested_nodes
        }}
    )


def mongo_remove_interest(job_name, clientid):
    global mongo_jobs
    interested_nodes = mongo_get_interest_workers(job_name)
    if interested_nodes is not None:
        if len(interested_nodes) > 0:
            interested_nodes.remove(clientid)
            mongo_jobs.db.jobs.update_one(
                {'job_name': job_name},
                {'$set': {
                    "interested_nodes": interested_nodes
                }}
            )
