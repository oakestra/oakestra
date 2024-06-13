import argparse
import csv
import os
import random

# import socket
import time

import docker
import requests

RESOURCE_ABSTRACTOR_ADDR = "http://0.0.0.0:11011"
CLUSTER_ADDR = f"{RESOURCE_ABSTRACTOR_ADDR}/api/v1/resources"
HOOKS_ADDR = f"{RESOURCE_ABSTRACTOR_ADDR}/api/v1/hooks"

DEFAULT_PROJECT = os.environ.get("DEFAULT_PROJECT") or "dummy_container"
DEFAULT_NETWORK = "root_orchestrator_default"
IMAGE = os.environ.get("IMAGE") or "stupidserver"

docker_client = docker.from_env()

containers = []
results = []
results.append(["i", "time_taken"])


def get_random_string(length):
    # choose from all lowercase letter
    result_str = "".join(random.choice("qwertyuiopasdfghjklzxcvbnm") for i in range(length))
    return result_str


def eval_operation(fn, *args, **kwargs):
    start = time.time()
    result = fn(*args, **kwargs)
    end = time.time()

    return result, end - start


def create_resource(resource):
    return requests.put(CLUSTER_ADDR, json=resource)


# def check_port(port):
#     with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
#         return s.connect_ex(("localhost", port)) != 0


def start_stupid_server(servers, sync_operation=False):
    for i in range(servers):
        print(f"{i} - creating stupid server...")
        random_port = random.randint(10000, 60000)
        environment = {
            "MY_PORT": random_port,
            "SERVICE_NAME": f"stupidserver-{i}",
            "RESOURCE_ABSTRACTOR_ADDR": "http://resource_abstractor:11011",
        }

        if sync_operation:
            environment["SYNC_OPERATION"] = True

        try:
            container = docker_client.containers.run(
                IMAGE,
                name=f"stupidserver-{i}",
                hostname=f"stupidserver-{i}",
                detach=True,
                ports={random_port: random_port},
                environment=environment,
                network=DEFAULT_NETWORK,
                labels={
                    "com.docker.compose.project": DEFAULT_PROJECT,
                    "com.docker.compose.service": f"stupidserver-{i}",
                },
            )
            containers.append(container)
        except Exception as e:
            # possible failure due to port already in use
            print(f"Failed to start container: {e}")

        time.sleep(1)

    print("Done creating stupid servers!")


def cleanup():
    print("Cleaning up...")

    requests.delete(CLUSTER_ADDR)

    response = requests.get(HOOKS_ADDR)
    response.raise_for_status()
    hooks = response.json()
    for hook in hooks:
        print(f"Deleting hook: {hook['_id']}")
        requests.delete(f"{HOOKS_ADDR}/{hook['_id']}")

    label = f"com.docker.compose.project={DEFAULT_PROJECT}"

    my_containers = docker_client.containers.list(filters={"label": label}, all=all)
    for container in my_containers:
        print(f"Stopping container: {container.name}")
        container.stop()
        container.remove()


def print_csv(mode="a", sync_operation=False):
    global results

    print("Writing to csv...")

    with open(f"results/creation_time_{int(sync_operation)}.csv", mode) as my_csv:
        csvWriter = csv.writer(my_csv, delimiter=",")
        csvWriter.writerows(results)
        results = []


def get_random_resource():
    return {
        "cluster_name": get_random_string(10),
        "cluster_location": get_random_string(10),
        "ip": get_random_string(10),
        "port": str(random.randint(10000, 60000)),
        "active_nodes": str(random.randint(1, 10)),
    }


def stress_test_hooks(resources_n, sync_operation=False):
    for i in range(resources_n):
        result, time_taken = eval_operation(create_resource, get_random_resource())
        print(f"{i} - Time taken: {time_taken}")
        results.append([i, time_taken])

        # we flush the results to csv every 100 iterations
        if i % 100 == 0:
            print_csv(sync_operation=sync_operation)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Stress test script.")
    parser.add_argument(
        "-r",
        "--resources",
        type=int,
        help="Number of resources to be created",
        default=0,
    )
    parser.add_argument(
        "-s",
        "--servers",
        type=int,
        help="Number of servers to be created",
        default=0,
    )
    parser.add_argument(
        "--sync",
        action="store_true",
        help="hook is sync instead of async",
        default=False,
    )

    args = parser.parse_args()
    resources = args.resources
    servers = args.servers
    sync_operation = args.sync

    cleanup()

    # reset csv file
    print_csv(mode="w+", sync_operation=sync_operation)

    if servers:
        start_stupid_server(servers, sync_operation=sync_operation)

    if resources:
        stress_test_hooks(resources, sync_operation=sync_operation)

    if results:
        print_csv(sync_operation=sync_operation)
