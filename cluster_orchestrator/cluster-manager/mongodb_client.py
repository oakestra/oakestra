import os
from flask_pymongo import PyMongo
from bson.objectid import ObjectId
import json
from datetime import datetime
from geometry import create_obfuscated_polygons_based_on_concave_hull
from shapely.geometry import mapping
import numpy as np

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

def mongo_upsert_node(obj):
    global app, mongo_nodes
    app.logger.info("MONGODB - upserting node...")
    json_node_info = json.loads(obj['node_info'])
    node_info_hostname = json_node_info.get('host')

    nodes = mongo_nodes.db.nodes
    # find node by hostname and if it exists, just upsert
    node_id = nodes.find_one_and_update({'node_info.host': node_info_hostname},
                                        {'$set': {
                                            'node_info': json_node_info,
                                            'node_address': obj.get('ip'),
                                            'node_subnet': obj.get('node_subnet'),
                                        }},
                                        upsert=True, return_document=True).get('_id')
    app.logger.info(node_id)
    return node_id


def mongo_find_node_by_id(node_id):
    global mongo_nodes
    return mongo_nodes.db.nodes.find_one(node_id)


def mongo_find_node_by_name(node_name):
    global mongo_nodes
    try:
        return mongo_nodes.db.nodes.find_one({'node_info.host': node_name})
    except Exception as e:
        return 'Error'


def mongo_find_node_by_id_and_update_cpu_mem(node_id, node_cpu_used, cpu_cores_free, node_mem_used,
                                             node_memory_free_in_MB, lat, long, public_ip, private_ip, router_rtt,
                                             vivaldi_vector, vivaldi_height, vivaldi_error, netem_delay):
    global app, mongo_nodes
    app.logger.info('MONGODB - update cpu and memory of worker node {0} ...'.format(node_id))
    # o = mongo.db.nodes.find_one({'_id': node_id})
    # print(o)

    time_now = datetime.now()
    app.logger.info(f"UPDATING NODE: {mongo_nodes.db.nodes.find_one(node_id)}")
    mongo_nodes.db.nodes.find_one_and_update(
        {'_id': ObjectId(node_id)},
        {'$set': {'current_cpu_percent': node_cpu_used, 'current_cpu_cores_free': cpu_cores_free,
                  'current_memory_percent': node_mem_used, 'current_free_memory_in_MB': node_memory_free_in_MB,
                  'last_modified': time_now, 'last_modified_timestamp': datetime.timestamp(time_now),
                  'lat': lat, 'long': long, 'public_ip': public_ip, 'private_ip': private_ip, 'router_rtt': router_rtt,
                  'vivaldi_vector': vivaldi_vector, 'vivaldi_height': vivaldi_height, 'vivaldi_error': vivaldi_error,
                  'netem_delay': netem_delay}},
        upsert=True)

    return 1


def find_one_edge_node():
    """Find first occurrence of edge nodes"""
    global mongo_nodes
    return mongo_nodes.db.nodes.find_one()


def find_all_nodes():
    global mongo_nodes
    return mongo_nodes.db.nodes.find()


def mongo_dead_nodes():
    print('looking for dead nodes')


def mongo_aggregate_node_information(TIME_INTERVAL):
    """ 1. Find all nodes"""
    """ 2. Aggregate cpu, memory, and more information of worker nodes"""

    global mongo_nodes

    cumulative_cpu = 0
    cumulative_cpu_cores = 0
    cumulative_memory = 0
    cumulative_memory_in_mb = 0
    number_of_active_nodes = 0
    # technology = []
    virtualization = []
    worker_names_coords = []
    nodes = find_all_nodes()
    for n in nodes:
        # print(n)

        # if it is not older than TIME_INTERVAL
        try:
            if n.get('last_modified_timestamp') >= (datetime.now().timestamp() - TIME_INTERVAL):
                cumulative_cpu += n.get('current_cpu_percent')
                cumulative_cpu_cores += n.get('current_cpu_cores_free')
                cumulative_memory += n.get('current_memory_percent')
                cumulative_memory_in_mb += n.get('current_free_memory_in_MB')
                number_of_active_nodes += 1
                for t in n.get('node_info').get('virtualization'):
                    virtualization.append(t) if t not in virtualization else virtualization

                # TODO: For research just send the actual coordinates of the worker nodes. In the future we want to
                # obfuscate these information
                name = n.get('node_info').get('host')
                lat = n.get('lat')
                long = n.get('long')
                worker_names_coords.append((name, float(lat), float(long)))
            else:
                print('Node {0} is inactive.'.format(n.get('_id')))
        except Exception as e:
            print("Problem during the aggregation of the data, skipping the node: ", str(n), " - because - ", str(e))

    jobs = mongo_find_all_jobs()
    for j in jobs:
        print(j)

    coords = [[lat, long] for _, lat, long in worker_names_coords]
    geo = create_obfuscated_polygons_based_on_concave_hull(coords)
    worker_groups = mapping(geo) if geo is not None else None
    return {'cpu_percent': cumulative_cpu, 'memory_percent': cumulative_memory,
            'cpu_cores': cumulative_cpu_cores, 'cumulative_memory_in_mb': cumulative_memory_in_mb,
            'number_of_nodes': number_of_active_nodes, 'jobs': jobs, 'virtualization': virtualization, 'more': 0,
            'worker_groups': worker_groups}


# ................. Job Operations .......................#
###########################################################

def mongo_upsert_job(job):
    print('insert/upsert requested job')
    job['system_job_id'] = job['_id']
    del job['_id']
    ## REMOVE ENTRY FROM DB
    result = mongo_jobs.db.jobs.find_one_and_update({'system_job_id': job['system_job_id']}, {'$set': job}, upsert=True,
                                                    return_document=True)  # if job does not exist, insert it
    result['_id'] = str(result['_id'])
    return result


def mongo_find_job_by_system_id(system_job_id):
    print('Find job by Id and return cluster.. and delete it...')
    # return just the assigned node of the job
    job_obj = mongo_jobs.db.jobs.find_one({'system_job_id': system_job_id})
    return job_obj


def mongo_find_job_by_id(id):
    print('Find job by Id')
    return mongo_jobs.db.jobs.find_one({'_id': ObjectId(id)})


def mongo_find_all_jobs():
    global mongo_jobs
    # list (= going into RAM) okey for small result sets (not clean for large data sets!)
    return list(mongo_jobs.db.jobs.find({}, {'_id': 0, 'system_job_id': 1, 'status': 1}))


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
            instance['host_port'] = port
            instance['worker_id'] = node.get('_id')
            break
    return mongo_jobs.db.jobs.update_one({'_id': ObjectId(job_id)},
                                         {'$set': {'status': status, 'instance_list': instance_list}})


def mongo_update_job_deployed(job_id, status, ns_ip, node_id):
    global mongo_jobs
    job = mongo_jobs.db.jobs.find_one({'_id': ObjectId(job_id)})
    instance_list = job['instance_list']
    for instance in instance_list:
        if str(instance.get('worker_id')) == str(node_id) and instance.get('namespace_ip') is '':
            instance['namespace_ip'] = ns_ip
            break
    return mongo_jobs.db.jobs.update_one({'_id': ObjectId(job_id)},
                                         {'$set': {'status': status, 'instance_list': instance_list}})
