import argparse
import csv
import random
import time

import requests
from kubernetes import client, config

CUSTOM_RESOURCES_ADDR = "http://0.0.0.0:11011/api/v1/custom-resources"
GROUP = "edu.oak"
VERSION = "v1"

api_crd = client.ApiextensionsV1Api()
api_crd_objects = client.CustomObjectsApi()


results1 = []
results2 = []
results1.append(["i", "creation_time"])
results2.append(["i", "creation_time", "retrieval_time", "deletion_time"])

fields = {
    "name": "string",
    "email": "string",
    "age": "integer",
    "address": "string",
    "salary": "integer",
    "education": "string",
    "experience": "integer",
    "skills": "array",
    "languages": "array",
    "bio": "string",
}


def get_random_string(length):
    # choose from all lowercase letter
    result_str = "".join(random.choice("qwertyuiopasdfghjklzxcvbnm") for i in range(length))
    return result_str


def generate_random_schema():
    schema = {"type": "object", "properties": {}}

    selected_fields = random.sample(fields.items(), k=random.randint(2, len(fields)))

    for field, field_type in selected_fields:
        schema["properties"][field] = {"type": field_type}
        if field_type == "array":
            schema["properties"][field]["items"] = {"type": "string"}

    return schema


def generate_crd(crd_name, schema):
    body = {
        "api_version": "apiextensions.k8s.io/v1",
        "kind": "CustomResourceDefinition",
        "metadata": client.V1ObjectMeta(**{"name": f"{crd_name}s.{GROUP}"}),
        "spec": client.V1CustomResourceDefinitionSpec(
            **{
                "group": GROUP,
                "names": {
                    "kind": crd_name,
                    "plural": f"{crd_name}s",
                },
                "scope": "Namespaced",
                "versions": [
                    client.V1CustomResourceDefinitionVersion(
                        **{
                            "name": VERSION,
                            "served": True,
                            "storage": True,
                            "schema": client.V1CustomResourceValidation(
                                **{
                                    "open_apiv3_schema": client.V1JSONSchemaProps(
                                        **{
                                            "type": "object",
                                            "properties": {
                                                "apiVersion": {"type": "string"},
                                                "kind": {"type": "string"},
                                                "metadata": {"type": "object"},
                                                "spec": schema,
                                            },
                                        }
                                    )
                                },
                            ),
                        }
                    )
                ],
            },
        ),
    }

    return body


def generate_crd_data_structure(kind, schema):
    return {
        "apiVersion": f"{GROUP}/{VERSION}",
        "kind": kind,
        "metadata": {"name": get_random_string(8)},
        "spec": generate_random_data_for_schema(schema),
    }


def generate_random_data_for_schema(schema):
    data = {}

    for field, field_type in schema["properties"].items():
        if field_type["type"] == "string":
            data[field] = get_random_string(10)
        elif field_type["type"] == "integer":
            data[field] = random.randint(1, 100)
        elif field_type["type"] == "array":
            data[field] = [get_random_string(10) for i in range(random.randint(1, 5))]

    return data


def create_resource(resource_type, data):
    data["resource_type"] = resource_type
    response = requests.post(CUSTOM_RESOURCES_ADDR, json=data)
    response.raise_for_status()

    return response.json()


def add_entry(resource_type, data):
    response = requests.post(f"{CUSTOM_RESOURCES_ADDR}/{resource_type}", json=data)
    response.raise_for_status()

    return response.json()


def get_entry(resource_type, entry_id):
    response = requests.get(f"{CUSTOM_RESOURCES_ADDR}/{resource_type}/{entry_id}")
    response.raise_for_status()

    return response.json()


def delete_entry(resource_type, entry_id):
    response = requests.delete(f"{CUSTOM_RESOURCES_ADDR}/{resource_type}/{entry_id}")
    response.raise_for_status()

    return response.json()


def eval_operation(fn, *args):
    start = time.time()
    result = fn(*args)
    end = time.time()

    return result, end - start


def test_custom_resources(custom_resources, entries):
    global results2, results1

    print(f"Creating {custom_resources} resource types with {entries} entries each.")
    for i in range(custom_resources):
        resource_type = get_random_string(6)
        schema = generate_random_schema()
        data = {"schema": schema}

        result, creation_time = eval_operation(create_resource, resource_type, data)
        assert result is not None

        results1.append([i, creation_time])

        print(f"\t{i} - created resource in {creation_time}")

        for j in range(entries):
            entry_data = generate_random_data_for_schema(schema)
            result, creation_time = eval_operation(add_entry, resource_type, entry_data)
            assert result is not None

            print(f"\t{j} - created object in {creation_time}")

            entry_id = result["_id"]
            assert entry_id is not None

            # _, retrieval_time = eval_operation(get_entry, resource_type, entry_id)
            # _, deletion_time = eval_operation(delete_entry, resource_type, entry_id)

            results2.append([i, creation_time, 0, 0])

            if len(results2) % 1000 == 0:
                print_csv(results2, "entries_0")
                results2 = []


def test_custom_resources_k(custom_resources, entries):
    global results2, results1

    print(f"Kube - Creating {custom_resources} resource types with {entries} entries each.")
    for i in range(custom_resources):
        resource_name = get_random_string(6)
        schema = generate_random_schema()
        crd = generate_crd(resource_name, schema)

        body = client.V1CustomResourceDefinition(**crd)
        result, creation_time = eval_operation(api_crd.create_custom_resource_definition, body)
        assert result is not None

        print(f"{i} - created resource in {creation_time}")
        results1.append([i, creation_time])
        time.sleep(1)

        for j in range(entries):

            data = generate_crd_data_structure(resource_name, schema)

            created_obj, creation_time = eval_operation(
                api_crd_objects.create_namespaced_custom_object,
                GROUP,
                VERSION,
                "default",
                f"{resource_name}s",
                data,
            )
            assert created_obj is not None

            # retrieved_obj, retrieval_time = eval_operation(
            #     api_crd_objects.get_namespaced_custom_object,
            #     GROUP,
            #     VERSION,
            #     "default",
            #     f"{resource_name}s",
            #     created_obj["metadata"]["name"],
            # )
            # assert retrieved_obj is not None

            # _, deletion_time = eval_operation(
            #     api_crd_objects.delete_namespaced_custom_object,
            #     GROUP,
            #     VERSION,
            #     "default",
            #     f"{resource_name}s",
            #     created_obj["metadata"]["name"],
            # )
            results2.append([i, creation_time, 0, 0])

            print(f"\t{j} - created object in {creation_time}")

            if len(results2) % 1000 == 0:
                print_csv(results2, "entries_1")
                results2 = []

        api_crd.delete_custom_resource_definition(f"{resource_name}s.{GROUP}")


def print_csv(results, filename="entries", mode="a+"):
    with open(f"results/{filename}.csv", mode) as my_csv:
        csvWriter = csv.writer(my_csv, delimiter=",")
        csvWriter.writerows(results)


def cleanup_kubernetes(group=GROUP):
    crds = api_crd.list_custom_resource_definition().items
    namespace = "default"

    for crd in crds:
        # Skip CRDs not in the specified group
        if crd.spec.group != group:
            continue

        versions = [v.name for v in crd.spec.versions]
        plural = crd.spec.names.plural

        for version in versions:
            try:
                api_crd_objects.delete_collection_namespaced_custom_object(
                    group=group, version=version, namespace=namespace, plural=plural
                )
            except Exception as e:
                print(
                    (
                        f"Error deleting objects for CRD {plural}.{group}/{version}",
                        f"in namespace {namespace}: {e}",
                    )
                )

        try:
            api_crd.delete_custom_resource_definition(crd.metadata.name)
        except Exception as e:
            print(f"Error deleting CRD {crd.metadata.name}: {e}")


def cleanup():
    print("Cleaning up custom resources...")
    cleanup_kubernetes()

    response = requests.delete(CUSTOM_RESOURCES_ADDR)
    response.raise_for_status()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Custom resources evaluation script.")
    parser.add_argument(
        "-r",
        "--resources",
        type=int,
        help="Number of resource types to create",
        default=1,
    )
    parser.add_argument(
        "-e",
        "--entries",
        type=int,
        help="Number of resource entries per resource to create",
        default=1,
    )
    parser.add_argument(
        "-k",
        "--kube",
        action="store_true",
        help="run for kubernetes",
        default=False,
    )

    args = parser.parse_args()
    resources = args.resources
    entries = args.entries
    kubernetes = args.kube

    cleanup()

    print_csv(results1, f"resources_{int(kubernetes)}", mode="w+")
    print_csv(results2, f"entries_{int(kubernetes)}", mode="w+")
    results1 = []
    results2 = []

    if kubernetes:
        config.load_kube_config()
    (
        test_custom_resources(resources, entries)
        if not kubernetes
        else test_custom_resources_k(resources, entries)
    )

    print_csv(results1, f"resources_{int(kubernetes)}")
    print_csv(results2, f"entries_{int(kubernetes)}")
