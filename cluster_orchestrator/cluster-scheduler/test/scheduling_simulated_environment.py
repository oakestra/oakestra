import random
import csv
import time
import unittest
from unittest.mock import MagicMock
import os
import sys
import geopy.distance
import requests
from util import create_job, prepare, find_ip_for_id, shuffle_measurements, find_id_for_ip, vivaldi_coordinate_dimension
import numpy as np

myPath = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, myPath + '/../')
import calculation
from vivaldi_coordinate import VivaldiCoordinate

casa_results = [["overhead", "setup", "result_id", "rtt_to_trg", "viv_trg_dist",
            "min_rtt_to_trg", "s2s_cap", "rtt_to_user", "viv_user_dist", "min_rtt_to_user", "s2u_cap", "s2u_dist", "s2s_dist"]]
nat_results = [["overhead", "setup", "result_id", "rtt_to_trg", "viv_trg_dist",
            "min_rtt_to_trg", "s2s_cap", "rtt_to_user", "viv_user_dist", "min_rtt_to_user", "s2u_cap", "s2u_dist", "s2s_dist"]]

# Number of experiments to run
experiments = 7000
# Number of scheduling runs per experiment
runs = 5
# Number of worker nodes in the simulated edge environment
nr_nodes = 100
# Number of update iterations in the Vivaldi Network Coordinate network
iterations = 100
# Number of neighbors used to update the node positions in the Vivaldi network
neighbors = 6
# Target location for Service-to-User scheduling with a geographic constraint
s2u_geo_location = "48.13189654815318, 11.585990847392225"
# Scheduling threshold for the Service-to-User constraint in kilometers
s2u_geo_threshold = 100
# Latency constraint for Service-to-User scheduling
s2u_lat_area = "munich"
# Scheduling threshold for the Service-to-User constraint in milliseconds
s2u_lat_threshold = 20
# ATTENTION: change target id in util.py accordingly to avoid false results
s2s_target_id = "6"
s2s_lat_threshold = 20
s2s_geo_threshold = 100


class SchedulingSimulatedEnvironment(unittest.TestCase):

    def test_benchmark_sla_alarm_with_s2u_s2s_constraints_single_viv(self):
        """
        Scheduling after SLA violation with job containing S2U amd S2S constraints
        """
        session = requests.Session()
        measurements, user_measurements, worker_list, worker_ip_map, vivs = prepare(session=session, nr_nodes=nr_nodes, iterations=iterations,
                                                                                    neighbors=neighbors,
                                                                                    dim=vivaldi_coordinate_dimension,
                                                                                    s2s_range_map={"id": s2s_target_id,
                                                                                                   "threshold": s2s_geo_threshold})


        for i in range(experiments):
            print(f"Experiment: {i+1}/{experiments}")
            nat_job = create_job()
            casa_job = create_job(ms_id="2", ms_name="service2", s2u_geo_location=s2u_geo_location, s2u_geo_threshold=s2u_geo_threshold,
                                  s2u_lat_threshold=s2u_lat_threshold, s2u_lat_area=s2u_lat_area,
                                  s2s_target_id=s2s_target_id,
                                  s2s_geo_threshold=s2s_geo_threshold, s2s_lat_threshold=s2s_lat_threshold)

            current_setup = nr_nodes
            for j in range(runs):
                app = MagicMock()
                app.logger.info = lambda x: None
                source_client_id = str(random.randint(1, len(worker_list)))
                # native scheduling
                nat_start = time.time()
                nat_res, nat_mes, nat_job = calculation.calculate(nat_job)
                nat_stop = time.time()
                nat_overhead = nat_stop - nat_start
                nat_overhead *= 1000
                if nat_res != "positive":
                    raise Exception()
                # casa scheduling
                source_client_ip = find_ip_for_id(source_client_id)
                casa_start = time.time()
                casa_res, casa_mes, casa_job, casa_ip_locations = calculation.calculate(casa_job, is_sla_violation=True, source_client_id=source_client_id, worker_ip_rtt_stats={"255.0.0.1": user_measurements[f"{source_client_ip},255.0.0.1"]}, session=session)
                casa_stop = time.time()
                casa_overhead = casa_stop - casa_start
                casa_overhead *= 1000
                if casa_res != "positive":
                    # raise Exception()
                    print("no suitable node")
                    continue
                # Evaluate casa
                casa_res_worker_id = casa_mes.get("_id")
                casa_data = self.get_results(casa_res_worker_id, measurements, user_measurements, vivs, worker_list, casa_ip_locations)
                if casa_data is None:
                    continue
                s2s_rtt, s2s_viv_dist, min_s2s_rtt, s2s_threshold, s2u_rtt, s2u_viv_dist, min_s2u_rtt, s2u_threshold, s2u_dist, s2s_dist = casa_data
                casa_results.append([casa_overhead, current_setup, casa_res_worker_id, s2s_rtt, s2s_viv_dist, min_s2s_rtt, s2s_threshold,
                                     s2u_rtt, s2u_viv_dist, min_s2u_rtt, s2u_threshold, s2u_dist, s2s_dist])
                # Evaluate native
                nat_res_worker_id = nat_mes.get("_id")
                nat_data = self.get_results(nat_res_worker_id, measurements, user_measurements, vivs, worker_list, casa_ip_locations)
                if nat_data is None:
                    continue
                s2s_rtt, s2s_viv_dist, min_s2s_rtt, s2s_threshold, s2u_rtt, s2u_viv_dist, min_s2u_rtt, s2u_threshold, s2u_dist, s2s_dist = nat_data
                nat_results.append(
                    [nat_overhead, current_setup, nat_res_worker_id, s2s_rtt, s2s_viv_dist, min_s2s_rtt,
                     s2s_threshold,
                     s2u_rtt, s2u_viv_dist, min_s2u_rtt, s2u_threshold, s2u_dist, s2s_dist])
            # Shuffle measurements to have different results in each run with a single Vivaldi network creation.
            worker_list, vivs, user_measurements, measurements = shuffle_measurements(worker_list, vivs, user_measurements, measurements)
        self.print_calculation_csv()
        # self.print_ip_locations(ip_location_list)


    def print_calculation_csv(self):
        global casa_results
        global nat_results
        import os
        x = os.getcwd()
        with open(f"results/casa_{nr_nodes}workers_{iterations}iterations_{neighbors}neighbors_{vivaldi_coordinate_dimension}dim.csv", "w", newline="") as f:
            csv_writer = csv.writer(f, delimiter=",")
            csv_writer.writerows(casa_results)
        casa_results = [["overhead", "setup", "result_id", "rtt_to_trg", "viv_trg_dist",
                    "min_rtt_to_trg", "s2s_cap", "rtt_to_user", "viv_user_dist", "min_rtt_to_user", "s2u_cap"]]
        f.close()

        with open(f"results/nat_{nr_nodes}workers_{iterations}iterations_{neighbors}neighbors_{vivaldi_coordinate_dimension}dim.csv", "w", newline="") as f:
            csv_writer = csv.writer(f, delimiter=",")
            csv_writer.writerows(nat_results)
        nat_results = [["overhead", "setup", "result_id", "rtt_to_trg", "viv_trg_dist",
                    "min_rtt_to_trg", "s2s_cap", "rtt_to_user", "viv_user_dist", "min_rtt_to_user", "s2u_cap"]]
        f.close()

    def print_ip_locations(self, ip_locations):
        with open("ip_locations.txt", "w") as f:
            str_res = str(ip_locations)
            str_res = str_res.replace("array(", "")
            str_res = str_res.replace(")", "")
            str_res = str_res.replace("\'", "\"")
            f.write(str_res)
            f.close()

    def read_ip_locations(self):
        import ast
        f = open("ip_locations.txt")
        ip_locations = f.read()
        ip_locations = ast.literal_eval(ip_locations)
        f.close()
        return ip_locations

    def get_results(self, res_worker_id, measurements, user_measurements, vivs, worker_list, ip_locations=None):
        # Find RTT to target worker
        res_ip = find_ip_for_id(res_worker_id)
        trgt_ip = find_ip_for_id(s2s_target_id)
        if res_ip != trgt_ip:
            if f"{trgt_ip},{res_ip}" in measurements:
                s2s_rtt = measurements.get(f"{trgt_ip},{res_ip}")
            elif f"{res_ip},{trgt_ip}" in measurements:
                s2s_rtt = measurements.get(f"{res_ip},{trgt_ip}")
        else:
            return None
        # Find Vivaldi dist to target worker
        s2s_viv_dist = vivs[int(s2s_target_id) - 1].distance(vivs[int(res_worker_id) - 1])

        # Build list of RTTs from each node to target to find minimal cluster RTT
        worker_rtts = []
        for k, v in measurements.items():
            src, trg = k.split(",")
            if trg != "0.0.0.0":
                src_id = find_id_for_ip(src)
                trg_id = find_id_for_ip(trg)
                if src_id == s2s_target_id or trg_id == s2s_target_id:
                    worker_rtts.append(v)
        min_s2s_rtt = min(worker_rtts)
        s2u_rtt = ""
        s2u_viv_dist = ""
        min_s2u_rtt = ""
        s2u_threshold = ""
        # Find RTT to user
        s2u_rtt = user_measurements[f"{res_ip},255.0.0.1"]
        # Find minimal RTT to user
        min_s2u_rtt = min([v for k, v in user_measurements.items() if k.split(",")[1] == "255.0.0.1"])
        # Find Vivaldi dist to user
        res_worker_viv = vivs[int(res_worker_id) - 1]
        user_coords = ip_locations["255.0.0.1"]
        user_viv = VivaldiCoordinate(vivaldi_coordinate_dimension)
        user_viv.vector = np.array(user_coords)
        s2u_viv_dist = res_worker_viv.distance(user_viv)
        s2s_threshold = s2s_lat_threshold * 1.2
        s2u_threshold = s2u_lat_threshold * 1.2

        # Geo dist to constriant area
        res_worker = [n for n in worker_list if n.get("_id") == res_worker_id][0]
        res_loc = [float(res_worker.get("lat")), float(res_worker.get("long"))]
        constraint_loc = [float(s2u_geo_location.split(",")[0]), float(s2u_geo_location.split(",")[1])]
        s2u_dist = geopy.distance.distance(res_loc, constraint_loc).km

        trg_worker = [n for n in worker_list if n.get("_id") == s2s_target_id][0]
        trg_loc = [float(trg_worker.get("lat")), float(trg_worker.get("long"))]
        s2s_dist = geopy.distance.distance(res_loc, trg_loc).km


        return s2s_rtt, s2s_viv_dist, min_s2s_rtt, s2s_threshold, s2u_rtt, s2u_viv_dist, min_s2u_rtt, s2u_threshold, s2u_dist, s2s_dist
