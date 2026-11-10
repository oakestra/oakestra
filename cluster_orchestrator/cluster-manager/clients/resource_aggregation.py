import logging
from itertools import zip_longest

from resource_abstractor_client import candidate_operations

logger = logging.getLogger("cluster_manager")

CLUSTER_FIELDS = {"_id", "ip", "port", "candidate_name", "candidate_location"}

counters = {
    "cpu_percent": 0,
    "memory_percent": 0,
    "vram_percent": 0,
    "gpu_temp": 0,
    "gpu_percent": 0,
}


def default_aggregator(w, acc, key):
    val = w.get(key)
    if val is None:
        return acc
    if isinstance(val, (int, float)):
        if key.endswith("_percent") or key.endswith("_average"):
            res = acc if acc is not None else 0.0
            return average_aggregator(w, acc, key, custom_counter=key)

        res = acc if acc is not None else 0
        return res + val
    if acc is None:
        acc = []
    if isinstance(val, list):
        acc.extend(val)
        return acc
    acc.append(val)
    return acc


def average_aggregator(w, acc, key, **kwargs):
    val = w.get(key)

    # Skip zero values when averaging
    if val is None or float(val) == 0:
        return acc

    custom_counter = kwargs.get("custom_counter")
    counter_key = custom_counter if custom_counter is not None else key

    if counter_key not in counters:
        counters[counter_key] = 0

    counters[counter_key] += 1
    n = counters[counter_key]

    if acc is None:
        acc = 0.0

    acc += (float(val) - acc) / n

    return acc


def csi_drivers_aggregator(w, acc=None):
    """Merge csi_drivers from a worker into a deduplicated list of driver names.

    Node Engine advertises drivers as objects: {csi_driver_name, csi_driver_endpoint}.
    After aggregation, the cluster reports a flat list of name strings to the root.
    """
    val = w.get("csi_drivers")
    if val is None:
        return acc
    if acc is None:
        acc = []
    if isinstance(val, list):
        for item in val:
            if isinstance(item, dict):
                name = item.get("csi_driver_name")
            elif isinstance(item, str):
                name = item
            else:
                continue
            if name and name not in acc:
                acc.append(name)
    return acc


def gpu_drivers_aggregator(w, acc=None):
    """Collect the worker's gpu_driver string into a deduplicated list.

    Workers send the singular key "gpu_driver" while the cluster reports "gpu_drivers".
    """
    if acc is None:
        acc = []
    driver = w.get("gpu_driver")
    if isinstance(driver, str) and driver not in ("", "-") and driver not in acc:
        acc.append(driver)
    return acc


def _worker_gpu_vram_free(w):
    free = w.get("gpu_vram_free")
    if isinstance(free, list):
        return sorted((float(v) for v in free), reverse=True)
    # Legacy workers do not report per-GPU free VRAM, assume an even split of total VRAM.
    vgpus = w.get("vgpus") or 0
    if vgpus > 0:
        return [float(w.get("vram") or 0) / vgpus] * int(vgpus)
    return []


def gpu_vram_free_aggregator(w, acc=None):
    """Element-wise max over the workers' descending free-VRAM lists.

    The root scheduler accepts a cluster if the top-N entries sum to >= vram and there are
    >= vgpus entries. This envelope never rejects a cluster that has a fitting worker, unlike
    summed vgpus/vram, which would accept a 2-GPU job in a cluster of 1-GPU nodes.
    """
    return [max(a, b) for a, b in zip_longest(acc or [], _worker_gpu_vram_free(w), fillvalue=0.0)]


# canonical resources are resources that are required by the system manager
# this dict contains {resource_name: aggregation_scheme}
# where aggregation scheme outlines how this resource should be aggregated.
# Every aggregation schema is a function that takes an accumulator and a worker
# and returns a new accumulator: acc, w -> acc
canonical_resources = {
    "cpu_percent": lambda w, acc=0.0: average_aggregator(w, acc, "cpu_percent"),
    "vcpus": lambda w, acc=0: default_aggregator(w, acc, "vcpus"),
    "memory_percent": lambda w, acc=0.0: average_aggregator(w, acc, "memory_percent"),
    "vram": lambda w, acc=0: default_aggregator(w, acc, "vram"),
    "vram_percent": lambda w, acc=0.0: average_aggregator(w, acc, "vram_percent"),
    "gpu_temp": lambda w, acc=0.0: average_aggregator(w, acc, "gpu_temp"),
    "gpu_drivers": gpu_drivers_aggregator,
    "gpu_percent": lambda w, acc=0.0: average_aggregator(w, acc, "gpu_percent"),
    "gpu_vram_free": gpu_vram_free_aggregator,
    "vgpus": lambda w, acc=0: default_aggregator(w, acc, "vgpus"),
    "memory": lambda w, acc=0: default_aggregator(w, acc, "memory"),
    "virtualization": lambda w, acc=None: default_aggregator(w, acc, "virtualization"),
    "supported_addons": lambda w, acc=None: default_aggregator(w, acc, "supported_addons"),
    "csi_drivers": lambda w, acc=None: csi_drivers_aggregator(w, acc),
    "active_nodes": lambda w, acc=0: acc if (w is None or w == {}) else acc + 1,
}


def aggregate_workers(workers):
    # reset global counters
    for key in counters.keys():
        counters[key] = 0

    result = {}

    if workers is None:
        return result

    for w in workers:
        # iterate over all worker resources, always collect canonical resources
        keys_to_process = set(w.keys()) | set(canonical_resources.keys())
        keys_to_process -= CLUSTER_FIELDS

        for key in keys_to_process:
            if key in canonical_resources:
                aggregator = canonical_resources[key]
                if key not in result:
                    result[key] = aggregator(w)
                else:
                    result[key] = aggregator(w, result[key])

            else:
                result[key] = default_aggregator(w, result.get(key), key)

    # add cumulative neutral values
    for key, agg in canonical_resources.items():
        if key not in result:
            result[key] = agg({})

    return result


def aggregate_info():
    # The resource abstractor only projects its canonical fields unless asked for more,
    # and the per-worker "gpu_driver" is not one of them.
    workers = candidate_operations.get_candidates(active=True, resources="gpu_driver")

    if workers is None:
        return {}

    result = aggregate_workers(workers)

    aggregation_per_arch = {}
    for w in workers:
        arch = w.get("architecture")
        if arch is None:
            continue
        aggregation_per_arch.setdefault(arch, []).append(w)

    result["aggregation_per_architecture"] = {
        arch: aggregate_workers(workers) for arch, workers in aggregation_per_arch.items()
    }

    return result
