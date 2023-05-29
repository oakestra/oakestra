import json
import random
import re
import os
import sys
from datetime import datetime
import time
from functools import partial
from itertools import islice
from unittest.mock import MagicMock
import pyproj
import requests_mock
from shapely.ops import transform
from shapely.geometry import Point, Polygon
from vivaldi_coordinate import VivaldiCoordinate
import matplotlib.pyplot as plt
import numpy as np

myPath = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, myPath + '/../')
import calculation

MUNICH = Polygon([[48.24819, 11.50406], [48.22807, 11.63521], [48.18093, 11.69083], [48.1369, 11.72242],
                  [48.07689, 11.68534], [48.06221, 11.50818], [48.13008, 11.38871], [48.15757, 11.36124],
                  [48.20107, 11.39077]])
GARCHING = Polygon([[48.26122, 11.59742], [48.27013, 11.68445], [48.21720, 11.65063], [48.23013, 11.59862]])
GERMANY = Polygon([[53.69039499952727, 7.19896078851138],[54.83252457729241, 9.022691264644848],[54.258998141984534, 13.17552331270781],
                   [51.13131284515075, 14.625718631079001], [50.29656693243139, 12.120835808437851], [48.69842804429054, 13.768785033859663],
                   [47.613790266506015, 12.208726433793686], [47.732154490129666, 7.638413915290528], [48.973220613985035, 8.209702980103422],
                   [49.5040219286023, 6.320054534953079]])
AREAS = {
    "munich": MUNICH,
    "garching": GARCHING,
    "germany": GERMANY # Used for testing different latency measures from requests within germany
}

# Adapt these values to change:
# the dimension of the Vivaldi Network Coordinates
vivaldi_coordinate_dimension = 2
# the target worker node for Service-to-Service scheduling
s2s_target_id = "6"

measurements = {}
user_measurements = {}
worker_list = []
worker_ip_map = {}


def create_job(app_id="1", app_name="app1", app_ns="test", ms_id="1", ms_name="service1", ms_ns="test",
               memory=100, vcpus=1, target_node=None,
               s2u_geo_location=None, s2u_geo_threshold=None, s2u_lat_area=None,
               s2u_lat_threshold=None, s2s_target_id=None, s2s_geo_threshold=None, s2s_lat_threshold=None):
    job = {
        "_id": 1,
        "applicationID": app_id,
        "application_name": app_name,
        "application_namespace": app_ns,
        "microserviceID": ms_id,
        "microservice_name": ms_name,
        "microservice_namespace": ms_ns,
        "virtualization": "docker",
        "memory": memory,
        "vcpus": vcpus,
        "constraints": [],
        "connectivity": []
    }
    lat_con = None
    geo_con = None
    if s2u_lat_area:
        lat_con = {
                "type": "latency",
                "area": s2u_lat_area,
                "threshold": s2u_lat_threshold,
                "rigidness": 0.5,
                "convergence_time": 60
            }
    if s2u_geo_location:
        geo_con = {
                "type": "geo",
                "location": s2u_geo_location,
                "threshold": s2u_geo_threshold,
                "rigidness": 0.5,
                "convergence_time": 60
            }
    if lat_con is not None and geo_con is None:
        job["constraints"].append(lat_con)
    elif lat_con is None and geo_con is not None:
        job["constraints"].append(geo_con)
    elif lat_con is not None and geo_con is not None:
        job["constraints"] += [lat_con, geo_con]

    if s2s_target_id:
        job["connectivity"] = [
            {
                "target_microservice_id": s2s_target_id,
                "con_constraints": [
                    {
                        "type": "latency",
                        "threshold": s2s_lat_threshold
                    },
                    {
                        "type": "geo",
                        "threshold": s2s_geo_threshold
                    }
                ]
            }
        ]

    if target_node is not None:
        job["target_node"] = target_node

    return job

################################################################################################
############################################## Start RTT #######################################
def build_user_rtts_map(nr_users, nr_workers, vivs):
    global user_measurements
    viv_coords = np.array([v.vector for v in vivs])
    user_ip = "255.0.0.1"
    worker_ip = "0.0.0.1"
    for i in range(nr_users):
        # user_position = generate_random(viv_polygon)
        coords = []
        for j in range(viv_coords.shape[1]):
            rnd = random.uniform(min(viv_coords[:, j]), max(viv_coords[:, j]))
            coords.append(rnd)
        user_viv = VivaldiCoordinate(vivaldi_coordinate_dimension)
        # user_viv.vector = np.array([*user_position.coords[0]])
        user_viv.vector = np.array(coords)
        co_viv = vivs[-1]
        print(co_viv.vector)
        print(user_viv.vector)
        # user_measurements[f"0.0.0.0,{user_ip}"] = co_viv.distance(user_viv)
        user_measurements[f"0.0.0.0,{user_ip}"] = random.uniform(5.5, 63.9)
        for j in range(nr_workers):
            worker_id = find_id_for_ip(worker_ip)
            worker_viv = vivs[int(worker_id) - 1]
            rnd = random.randint(0,1)
            if rnd == 0:
               user_measurements[f"{worker_ip},{user_ip}"] = worker_viv.distance(user_viv) * 1.2
            else:
               user_measurements[f"{worker_ip},{user_ip}"] = worker_viv.distance(user_viv) * 0.8
            # user_measurements[f"{worker_ip},{user_ip}"] = random.uniform(5.5, 50.9)
            worker_ip = increment_ip(worker_ip)

        user_ip = increment_ip(user_ip)

    return user_measurements


def build_vivaldi_with_co(nodes, iterations, neighbors, dim, mesh=False):
    vivs = [VivaldiCoordinate(dim) for i in range(nodes)]
    co = VivaldiCoordinate(dim)
    co.vector = np.zeros(dim)
    vivs.append(co)
    if mesh:
        latencies, node_rtts = build_mesh(nodes, iterations, "mesh.txt", dim, neighbors)
    else:
        latencies, node_rtts = build_rtt_measures_for_each_node(nodes, "uking-t.txt", with_co=True, uking=True)
    # {1: [(2, xx), (3, xx),...], 2: [(1, xx), (3, xx),...], ...}
    # latencies: {'1,0': xx, '1,2': xx, '1,3': xx, '1,4': xx,
    #             '2,0': xx, '2,3': xx, '2,4': xx,
    #             '3,0': xx, '3,4': xx,
    #             '4,0': xx}
    # node_rtts: {'1': [('0', xx), ('2', xx), ('3', xx), ('4', xx)],
    #             '2': [('0', xx), ('1', xx), ('3', xx), ('4', xx)],
    #             '3': [('0', xx), ('1', xx), ('2', xx), ('4', xx)],
    #             '4': [('0', xx), ('1', xx), ('2', xx), ('3', xx)]}
    mres = []
    mses = []
    for i in range(1, iterations + 1):
        for key, value in node_rtts.items():
            # CO is passive member and doesn't update its postition. Only the workers update their position to other
            # workers and the CO
            if key == "0":
                continue
            # key=1, value=[(2,12.0), (3,52.21), ...]
            # Neighbor Selection 1: Shuffle values to ping random x nodes
            target_nodes = [v for v in value if v[0] is not "0"]
            random.shuffle(target_nodes)
            # Neighbor Selection 2: ping closest neighbors
            # value = sorted(value, key=lambda x: x[1])
            # Neighbor Selection 3: ping half close and half distant neighbors
            # value = sorted(value, key=lambda x: x[1])
            # half = math.floor(neighbors/2)
            # value = value[:half] + value[-half:]
            for dst, rtt in target_nodes[:neighbors] + [v for v in value if v[0] is "0"]:
                if float(rtt) < 0: continue
                vivs[int(key) - 1].update(float(rtt), vivs[int(dst) - 1])

        if i % 10 == 0:
            print(f"{i}/{iterations}")
        #    plot_vivs(vivs)
        #    plt.show()

        # Evaluate estimations
        # latencies = build_stats_for_n_nodes(nodes, "uking-t.txt", with_co=True)
        # {'1,2': 'xx.yyy', '1,3': 'xx.yyy', '2,3': 'xx.yy'}
        # latencies = build_stats_for_n_nodes(nodes, "fsoc_lat_200_nodes_2.txt")

        total_relative_error = 0
        total_squared_error = 0
        for key, value in latencies.items():
            src, dst = key.split(",")
            latency = latencies[key]
            if float(latency) < 0:
                continue
            estimation = vivs[int(src) - 1].distance(vivs[int(dst) - 1])
            absolute_error = abs(latency - estimation)
            relative_error = absolute_error / latency
            total_relative_error += relative_error
            squared_error = (latency - estimation) ** 2
            total_squared_error += squared_error
            # if i == iterations-1:
            #    print(f"{src}->{dst}: Est: {round(estimation,2)} Lat: {round(latency,2)} AE: {round(absolute_error,2)} RE: {round(relative_error,4 )}")
        mre = total_relative_error / len(latencies)
        mres.append(mre)
        mse = total_squared_error / len(latencies)
        mses.append(mse)
        # print(f"MRE={round(mre,2)}")
    return mses, mres, vivs, latencies


def get_random_node_id(db):
    nodes = db.nodes.find()
    node_ids = [n.get("_id") for n in nodes]
    return str(random.choice(node_ids))


def build_mesh(size, iterations, file, dim, neighbors):
    vivs = [VivaldiCoordinate(dim) for i in range(size)]
    latencies, node_rtts = build_rtt_measures_for_each_node(size, file, with_co=False, uking=False)
    # plot_vivs(vivs)
    # plt.show()
    for i in range(iterations):
        for key, value in node_rtts.items():
            # key=1, value=[(2,12.0), (3,52.21), ...]
            # Neighbor Selection 1: Shuffle values to ping random x nodes
            random.shuffle(value)
            # Neighbor Selection 2: ping closest neighbors
            # value = sorted(value, key=lambda x: x[1])
            # Neighbor Selection 3: ping half close and half distant neighbors
            # value = sorted(value, key=lambda x: x[1])
            # half = math.floor(neighbors/2)
            # value = value[:half] + value[-half:]
            for dst, rtt in value[:neighbors]:
                vivs[int(key) - 1].update(float(rtt), vivs[int(dst) - 1])

        # if i % 10 == 0:
        #    plot_vivs(vivs)
        #    plt.show()

        # Evaluate estimations
        total_relative_error = 0
        for key, value in latencies.items():
            src, dst = key.split(",")
            latency = latencies[key]
            estimation = vivs[int(src) - 1].distance(vivs[int(dst) - 1])
            absolute_error = abs(latency - estimation)
            relative_error = absolute_error / latency
            total_relative_error += relative_error
            # if i == 99:
            #    print(f"{src}->{dst}: {round(estimation,2)} {round(latency,2)} AE: {round(absolute_error,2)} RE: {round(relative_error,4 )}")
        # if i % 10 == 0:
        #    mre = total_relative_error/len(latencies)
        #    print(f"MRE={round(mre,2)}")

    return latencies, node_rtts


def build_stats_for_n_nodes(n, file, with_co=False, uking=False):
    """
    return: {'1,2': 'xx.yyy', '1,3': 'xx.yyy', '2,3': 'xx.yy'}
    """
    # f = open("uking-t.txt", "r")
    f = open(file, "r")
    ctr = 1
    statistics = []

    if n > 1:
        for line in f:
            # src = line.split("->")[0]
            src = int(line.split(",")[0])
            if src == ctr:
                line = line.strip()
                statistics.append(line)
                if ctr != n:
                    for s in islice(f, n - 1 - ctr):
                            statistics.append(s.replace("\n", "").strip())
                ctr += 1
            if ctr == n:
                break

    f.close()

    if with_co:
        # Nodes to CO RTTs
        co_rtts = [random.randint(10, 30) for i in range(n)]
        # Append nth node to statistics, because its missing if we add the CO
        statistics.append(f"{n},0 {random.randint(10000, 30000)}")

    # statistics: ['1,2 xxxxx', '1,3 xxxxx', ..., '1,n xxxxx',
    # '2,3 16572, '2,4 29828.5',..., 2,n xxxxx,
    #  ...
    # 'n-1,n xxx']
    measurements = {}
    for stat in statistics:
        src = stat.split(",")[0]
        dst_rtt = stat.split(",")[1]
        dst = dst_rtt.split(" ")[0]
        rtt = dst_rtt.split(" ")[1]
        if with_co:
            # add node to CO
            measurements[f"{src},0"] = co_rtts[int(src) - 1]
        # Add /1000 if using uking dataset because latencies are in microseconds?
        if uking:
            measurements[f"{src},{dst}"] = float(rtt) / 1000
        else:
            measurements[f"{src},{dst}"] = float(rtt)

    return measurements


def add_missing_measurements(measurements):
    """
    input: {'1,2': 'xx.yyy', '1,3': 'xx.yyy', '2,3': 'xx.yy'}
    output: {'1,2': 'xx.yyy', '1,3': 'xx.yyy', '2,3': 'xx.yy', '2,1': 'xx.yyy', '3,1': 'xx.yyy', '3,2': 'xx.yy'}
    """
    complete_measurements = measurements.copy()
    for key, value in measurements.items():
        src, dst = key.split(",")
        if f"{dst},{src}" not in complete_measurements.keys():
            complete_measurements[f"{dst},{src}"] = value

    return complete_measurements


def build_rtt_measures_for_each_node(n, file, with_co=False, uking=False):
    """
    return: {1: [(2, xx), (3, xx),...],
             2: [(1, xx), (3, xx),...],
             ...}
    """
    unidirectional_rtts = build_stats_for_n_nodes(n, file, with_co, uking)
    bidirectional_rtts = add_missing_measurements(unidirectional_rtts)
    i_measures = {}
    for key, value in bidirectional_rtts.items():
        src, dst = key.split(",")
        i_measures.setdefault(src, []).append((dst, value))

    # sort values lists
    for value in i_measures.values():
        value.sort(key=lambda x: int(x[0]))

    return (unidirectional_rtts, i_measures)


def plot_vivs(vivs):
    is_3d = vivs[0].vector.shape[0] == 3
    xs = []
    ys = []
    zs = []
    for viv in vivs:
        xs.append(viv.vector[0])
        ys.append(viv.vector[1])
        if is_3d:
            zs.append(viv.vector[2])
    if not is_3d:
        plt.scatter(xs, ys)
    else:
        fig = plt.figure()
        ax = fig.add_subplot(projection='3d')
        ax.scatter(xs, ys, zs)


def build_worker_rtt_mapping(nr_workers, worker_ip_map, with_co=True, uking=True):
    global measurements
    f = open("uking-t.txt", "r")
    ctr = 1
    statistics = []
    if nr_workers > 1:
        for line in f:
            # src = line.split("->")[0]
            src = int(line.split(",")[0])
            if src == ctr:
                line = line.strip()
                statistics.append(line)
                if ctr != nr_workers:
                    for s in islice(f, nr_workers - 1 - ctr):
                        statistics.append(s.replace("\n", "").strip())
                ctr += 1
            if ctr == nr_workers:
                break
    f.close()

    if with_co:
        # Nodes to CO RTTs
        co_rtts = [random.randint(10, 30) for i in range(nr_workers)]
        # Append nth node to statistics, because its missing if we add the CO
        statistics.append(f"{nr_workers},0 {random.randint(10000, 30000)}")

    # statistics: ['1,2 xxxxx', '1,3 xxxxx', ..., '1,n xxxxx',
    # '2,3 16572, '2,4 29828.5',..., 2,n xxxxx,
    #  ...
    # 'n-1,n xxx']
    for stat in statistics:
        src = stat.split(",")[0]
        src_ip = worker_ip_map[src]
        dst_rtt = stat.split(",")[1]
        dst = dst_rtt.split(" ")[0]
        dst_ip = worker_ip_map[dst]
        rtt = dst_rtt.split(" ")[1]
        if with_co:
            # add node to CO
            measurements[f"{src_ip},0.0.0.0"] = co_rtts[int(src) - 1]
        # Add /1000 if using uking dataset because latencies are in microseconds
        if uking:
            measurements[f"{src_ip},{dst_ip}"] = float(rtt) / 1000
        else:
            measurements[f"{src_ip},{dst_ip}"] = float(rtt)

    return measurements


############################################## End RTT #########################################
################################################################################################

def find_ip_for_id(node_id):
    ip = "0.0.0.0"
    for _ in range(int(node_id)):
        ip = increment_ip(ip)
    return ip

def increment_ip(ip):
    # xx.xx.xx.xx
    fst, snd, trd, fth = ip.split(".")
    if int(fth) < 255:
        return f"{fst}.{snd}.{trd}.{int(fth) + 1}"
    else:
        if int(trd) < 255:
            return f"{fst}.{snd}.{int(trd) + 1}.{0}"
        else:
            if int(snd) < 255:
                return f"{fst}.{int(snd) + 1}.{0}.{0}"
            else:
                if int(fst) < 255:
                    return f"{int(fst) + 1}.{0}.{0}.{0}"
                else:
                    print("No addresses left")
                    return -1

def find_id_for_ip(ip):
    node_id = 1
    while ip != "0.0.0.1":
        node_id += 1
        ip = decrement_ip(ip)
    return str(node_id)

def decrement_ip(ip):
    # xx.xx.xx.xx
    fst, snd, trd, fth = ip.split(".")
    if int(fth) > 0: #xx.xx.xx.21 -> xx.xx.xx.20
        return f"{fst}.{snd}.{trd}.{int(fth) - 1}"
    else: # xx.xx.xx.0
        if int(trd) > 0:
            return f"{fst}.{snd}.{int(trd) - 1}.{255}" # xx.xx.13.00 -> xx.xx.12.255
        else:
            if int(snd) > 0:
                return f"{fst}.{int(snd) - 1}.{255}.{255}" # xx.123.00.00 -> xx.122.255.5255
            else:
                if int(fst) > 0:
                    return f"{int(fst) - 1}.{255}.{255}.{255}"
                else:
                    print("No addresses left")
                    return -1


def plot_areas(workers, s2u, s2s):
    plt.figure(figsize=(16, 16), dpi=80)
    for w in workers:
        lat = float(w.get("lat"))
        long = float(w.get("long"))
        plt.scatter(lat, long, color="black")
    # Plot S2U
    xs, ys = s2u.exterior.xy
    plt.plot(xs, ys, color="red")
    # Plot S2S
    xs, ys = s2s.exterior.xy
    plt.plot(xs, ys, color="blue")
    # Plot Germany
    xs, ys = GERMANY.exterior.xy
    plt.plot(xs, ys, color="black")
    # Plot Munich
    xs, ys = MUNICH.exterior.xy
    plt.plot(xs, ys)
    plt.savefig(f"areas_{len(workers)}_workers.png")



def generate_random(polygon):
    minx, miny, maxx, maxy = polygon.bounds
    pnt = Point(random.uniform(minx, maxx), random.uniform(miny, maxy))
    return pnt


def buffer_in_meters(hull, meters):
    # Shapely knows nothing about the units. Therefore, when calling the buffer(x) method, shapely will buffer
    # the coordinates by x units. To buffer in meters, we first need to reproject the polygon into a Coordinate
    # Reference System (CRS) that uses meters.
    proj_meters = pyproj.Proj('epsg:3857')
    proj_latlng = pyproj.Proj('epsg:4326')

    project_to_meters = partial(pyproj.transform, proj_latlng, proj_meters)
    project_to_latlng = partial(pyproj.transform, proj_meters, proj_latlng)

    hull_meters = transform(project_to_meters, hull)

    buffer_meters = hull_meters.buffer(meters)
    buffer_latlng = transform(project_to_latlng, buffer_meters)
    return buffer_latlng


################################################################################################
############################################## Start Mocks #####################################
def register_reponse_mock(session):
    matcher = re.compile("http://[0-9]+.[0-9]+.[0-9]+.[0-9]+:[0-9]+/monitoring/ping")
    # session = requests.Session()
    adapter = requests_mock.Adapter()
    session.mount('http://', adapter)
    adapter.register_uri('POST', matcher, json=post_ping_callback)

def post_ping_callback(request, context):
    global user_measurements
    context.status_code = 200
    src = request.hostname
    dsts = json.loads(request.json())
    results = {}
    for dst in dsts:
        if dst.split(".")[0] == "255":
            results[dst] = user_measurements[f"{src},{dst}"]
        else:
            results[dst] = measurements[f"{src},{dst}"]

    # sleep to simulate ping duration
    # time.sleep(3)
    return results

def find_node_by_name(name):
    global worker_list
    for worker in worker_list:
        if name == worker.get("node_info").get("host"):
            return worker
    return "Worker not found"

def find_node_by_id(id):
    global worker_list
    for worker in worker_list:
        if id == worker.get("_id"):
            return worker
    return "Worker not found"

def find_job(app_id, service_id):
    job = {
        "_id": 1,
        "system_job_id": 1,
        "applicationID": str(app_id),
        "application_name": "app1",
        "application_namespace": "test",
        "microserviceID": str(service_id),
        "microservice_name": "service1",
        "microservice_namespace": "test",
        "virtualization": "docker",
        "memory": 100,
        "vcpus": 1,
        "instance_list": [
            {
                "instance_number": 0,
                "instance_ip": f"0.0.0.{s2s_target_id}",
                "cluster_id": str(1),
                "worker_id": s2s_target_id,
                "host_ip": f"0.0.0.{s2s_target_id}",
                "host_port": 50011
            }
        ],
        "constraints": [
            {
                "type": "latency",
                "area": "germany",
                "threshold": 100,
                "rigidness": 0.5,
                "convergence_time": 60
            },
            {
                "type": "geo",
                "location": "48.19349,11.63067",
                "threshold": 50,
                "rigidness": 0.5,
                "convergence_time": 60
            }
        ],
        "connectivity": []
    }
    return job

def vivaldi_data():
    global worker_list
    viv_data = []
    for w in worker_list:
        viv_data.append({"_id": str(w.get("_id")), "vivaldi_vector": w.get("vivaldi_vector"),
                         "vivaldi_height": w.get("vivaldi_height"), "vivaldi_error": w.get("vivaldi_error")})
    return viv_data

def geo_data():
    global worker_list
    viv_data = []
    for w in worker_list:
        viv_data.append({"_id": str(w.get("_id")), "lat": w.get("lat"), "long": w.get("long")})
    return viv_data

def parallel_ping(target_ips, node_id):
    global worker_ip_map
    results = {}
    node_ip = worker_ip_map[str(node_id)]
    for ip in target_ips:
        results[ip] = user_measurements[f"{node_ip},{ip}"]
    # sleep to simulate ping duration
    # time.sleep(3)
    return results
############################################## End Mocks #####################################
################################################################################################


# Prepare required data and mocks for test
def prepare(session, nr_nodes, iterations, neighbors, dim, s2u_range_map=None, s2s_range_map=None):
    global user_measurements
    user_measurements = {}
    global measurements
    measurements = {}
    global worker_ip_map
    worker_ip_map = {}
    global worker_list
    worker_list = []
    global vivs
    vivs = []
    # Create Vivaldi network
    mses, mres, vivs, latencies = build_vivaldi_with_co(nodes=nr_nodes, iterations=iterations, neighbors=neighbors, dim=dim)
    print(mres)
    # Create worker db entries
    worker_list, worker_ip_map = db_gen(num=nr_nodes, vivaldi_network=vivs, s2u_range_map=s2u_range_map, s2s_range_map=s2s_range_map, all_in_area=False)
    # Create mocks
    register_reponse_mock(session)
    calculation.mongo_find_node_by_name = MagicMock(side_effect=find_node_by_name)
    calculation.mongo_find_all_active_nodes = MagicMock(return_value=worker_list)
    calculation.mongo_find_all_nodes = MagicMock(return_value=worker_list)
    calculation.mongo_get_vivaldi_data = MagicMock(side_effect=vivaldi_data)
    calculation.mongo_get_geolocation_data = MagicMock(side_effect=geo_data)
    calculation.mongo_find_job_by_microservice_id = MagicMock(side_effect=find_job)
    calculation.mongo_find_node_by_id = MagicMock(side_effect=find_node_by_id)
    calculation.parallel_ping = MagicMock(side_effect=parallel_ping)
    # Create S2U and S2S latency measurements
    measurements = build_worker_rtt_mapping(nr_workers=nr_nodes, worker_ip_map=worker_ip_map)
    user_measurements = build_user_rtts_map(nr_users=1, nr_workers=nr_nodes, vivs=vivs)

    return measurements, user_measurements, worker_list, worker_ip_map, vivs


def create_worker(id, host, node_address, cpu, mem, location, viv):
    return {
        '_id': id,
        "node_info": {
            "host": host,
            "ip": "127.0.1.1",
            "uname": ["worker1"],
            "cpu_count_physical": 2,
            "cpu_count_total": 4,
            "port": "3001",
            "virtualization": ["docker"]
        },
        "node_address": node_address,
        "node_subnet": "0.0.0.0",
        "current_cpu_cores_free": cpu,
        "current_cpu_percent": 10.0,
        "current_free_memory_in_MB": mem,
        "current_memory_percent": 12.6,
        "last_modified": datetime.now(),
        "last_modified_timestamp": time.time(),
        "lat": location.x,
        "long": location.y,
        "private_ip": "0.0.0.0",
        "public_ip": "0.0.0.0",
        "router_rtt": "0.426",
        "vivaldi_error": viv.error,
        "vivaldi_height": viv.height,
        "vivaldi_vector": viv.vector
    }

def db_gen(num, vivaldi_network, s2u_range_map=None, s2s_range_map=None, all_in_area=True):
    worker_list = []
    worker_ip_map = {"0": "0.0.0.0"}
    node_address = "0.0.0.1"
    # Location in range of S2U geo constraint
    if s2u_range_map is not None:
        s2u_lat, s2u_long = s2u_range_map.get("loc").split(",")
        s2u_threshold = float(s2u_range_map.get("threshold"))
        s2u_point = Point(float(s2u_lat), float(s2u_long))
        s2u_target = buffer_in_meters(s2u_point, s2u_threshold * 1000)
    # Location in range of S2S geo constraint
    if s2s_range_map is not None:
        s2s_id = int(s2s_range_map.get("id"))
        s2s_threshold = float(s2s_range_map.get("threshold"))
    if all_in_area:
        for i in range(1, num+1):
            vcpus = random.randint(4, 12)
            mem = random.randint(100, 1000)
            location = generate_random(MUNICH)
            viv = vivaldi_network[i-1]
            elem = create_worker(id=str(i), host=f"W{i}", node_address=node_address, cpu=vcpus, mem=mem, location=location, viv=viv)
            worker_ip_map[f"{i}"] = node_address
            node_address = increment_ip(node_address)
            worker_list.append(elem)
    else:
        for i in range(1, num+1):
            if i == 1:
                # Locate first node in munich
                vcpus = 1
                mem = 1
                location = generate_random(MUNICH)
                if s2s_range_map is not None:
                    s2s_lat = location.x
                    s2s_long = location.y
                    s2s_point = Point(s2s_lat, s2s_long)
                    s2s_target = buffer_in_meters(s2s_point, s2s_threshold * 1000)
            else:
                if i <= num * 0.5:  # Create some workers at constraint locations
                    vcpus = random.randint(4,5)
                    mem = random.randint(100,1000)
                    if s2u_range_map is not None and s2s_range_map is None:
                        location = generate_random(s2u_target)
                    elif s2u_range_map is None and s2s_range_map is not None:
                        location = generate_random(s2s_target)
                    elif s2u_range_map is not None and s2s_range_map is not None:
                        if i <= num * 0.25:  # Create some workers close to the S2U location
                            location = generate_random(s2u_target)
                        elif num * 0.25 < i <= num * 0.5:  # Create some workers close to the S2U location
                            location = generate_random(s2s_target)
                    else:  # Create some workers distributed across germany
                        location = generate_random(GERMANY)
                elif num * 0.5 < i <= num * 0.55:  # Create some workers with too low nem
                    location = generate_random(GERMANY)
                    vcpus = random.randint(4,5)
                    mem = 0
                elif num * 0.55 < i <= num * 0.6:  # Create some workers with too low cpu
                    location = generate_random(GERMANY)
                    vcpus = 0
                    mem = random.randint(100, 1000)
                else:
                    vcpus = random.randint(4, 5)
                    mem = random.randint(100, 1000)
                    location = generate_random(GERMANY)

            viv = vivaldi_network[i-1]
            elem = create_worker(id=str(i), host=f"W{i}", node_address=node_address, cpu=vcpus, mem=mem, location=location, viv=viv)
            worker_ip_map[f"{i}"] = node_address
            node_address = increment_ip(node_address)
            worker_list.append(elem)

    if s2u_range_map is not None and s2s_range_map is not None:
        plot_areas(worker_list, s2u_target, s2s_target)
    return worker_list, worker_ip_map


def shuffle_measurements(worker_list, vivs, user_measurements, measurements):
    node_ids = [int(e.get("_id")) for e in worker_list]
    shuffled_node_ids = node_ids.copy()
    random.shuffle(shuffled_node_ids)

    # assign new viv+geo coords and cpu+mem for workers
    lats = [float(n.get("lat")) for n in worker_list]
    longs = [float(n.get("long")) for n in worker_list]
    cpus = [int(n.get("current_cpu_cores_free")) for n in worker_list]
    mems = [int(n.get("current_free_memory_in_MB")) for n in worker_list]
    for k, worker in enumerate(worker_list):
        worker["lat"] = lats[shuffled_node_ids[k] - 1]
        worker["long"] = longs[shuffled_node_ids[k] - 1]
        # worker["current_cpu_cores_free"] = cpus[shuffled_node_ids[k] - 1]
        # worker["current_free_memory_in_MB"] = mems[shuffled_node_ids[k] - 1]
        worker["vivaldi_vector"] = vivs[shuffled_node_ids[k] - 1].vector
        worker["vivaldi_error"] = vivs[shuffled_node_ids[k] - 1].error
        worker["vivaldi_height"] = vivs[shuffled_node_ids[k] - 1].height
    new_vivs = []
    for id in shuffled_node_ids:
        new_vivs.append(vivs[id-1])
    vivs = new_vivs.copy()
    co = VivaldiCoordinate(vivaldi_coordinate_dimension)
    vivs.append(co)

    new_user_measurements = {}
    for k, v in user_measurements.items():
        src, trg = k.split(",")
        if src != "0.0.0.0":
            src_id = find_id_for_ip(src)
            new_src_id = shuffled_node_ids[int(src_id) - 1]
            new_src = find_ip_for_id(new_src_id)
            if f"{new_src},{trg}" in user_measurements:
                new_rtt = user_measurements[f"{new_src},{trg}"]
            elif f"{trg},{new_src}" in user_measurements:
                new_rtt = user_measurements[f"{trg},{new_src}"]
            new_user_measurements[f"{src},{trg}"] = new_rtt
        else:
            new_user_measurements[f"{src},{trg}"] = v

    user_measurements = new_user_measurements.copy()

    # assign new measurements
    new_measurements = {}

    for k, v in measurements.items():
        src, trg = k.split(",")
        src_id = find_id_for_ip(src)
        new_src_id = shuffled_node_ids[int(src_id) - 1]
        new_src = find_ip_for_id(new_src_id)
        if trg != "0.0.0.0":
            trg_id = find_id_for_ip(trg)
            new_trg_id = shuffled_node_ids[int(trg_id) - 1]
            new_trg = find_ip_for_id(new_trg_id)
            if f"{new_src},{new_trg}" in measurements:
                new_rtt = measurements[f"{new_src},{new_trg}"]
            elif f"{new_trg},{new_src}" in measurements:
                new_rtt = measurements[f"{new_trg},{new_src}"]
        else:
            new_rtt = measurements[f"{new_src},{trg}"]
        new_measurements[f"{src},{trg}"] = new_rtt
    measurements = new_measurements.copy()

    return worker_list, vivs, user_measurements, measurements