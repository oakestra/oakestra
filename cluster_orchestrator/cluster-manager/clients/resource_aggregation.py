from datetime import datetime, timedelta

from resource_abstractor_client import candidate_operations

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
        return acc + val
    if acc is None:
        acc = []
    if isinstance(val, list):
        acc.extend(val)
        return acc
    acc.append(val)
    return acc


def average_aggregator(w, acc, key, **kwargs):
    custom_counter = kwargs.get("custom_counter")
    counter_key = custom_counter if custom_counter is not None else key

    if counter_key not in counters:
        counters[counter_key] = 0

    val = w.get(key, None)
    if val is None:
        return acc

    if custom_counter is not None:
        if w.get(custom_counter, None) is None:
            increment = 0
        else:
            increment = w.get(custom_counter)
    else:
        increment = 1

    if acc is None:
        acc = 0.0

    if increment > 0:
        counters[counter_key] += increment

    if counters[counter_key] == 0:
        return acc

    acc -= (acc * increment) / counters[counter_key]
    acc += (float(val) * increment) / counters[counter_key]
    return acc

    return acc


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
    "gpu_drivers": lambda w, acc=None: default_aggregator(w, acc, "gpu_drivers"),
    "gpu_percent": lambda w, acc=0.0: average_aggregator(w, acc, "gpu_percent"),
    "vgpus": lambda w, acc=0: default_aggregator(w, acc, "vgpus"),
    "memory": lambda w, acc=0: default_aggregator(w, acc, "memory"),
    "virtualization": lambda w, acc=None: default_aggregator(w, acc, "virtualization"),
    "supported_addons": lambda w, acc=None: default_aggregator(w, acc, "supported_addons"),
    "active_nodes": lambda w, acc=0: 1 if w is None else acc + 1,
}


def aggregate_workers(workers):
    # reset global counters
    for key in counters.keys():
        counters[key] = 0

    result = {}
    for w in workers:
        for key, value in w.items():
            if key in canonical_resources.keys():
                if key not in result:
                    result[key] = canonical_resources[key](w)
                else:
                    result[key] = canonical_resources[key](w, result.get(key))
                continue

    # add cumulative neutral values
    for key, agg in canonical_resources.items():
        if key not in result:
            result[key] = agg({})

    return result


def aggregate_info(time_interval):
    cutoff = datetime.now() - timedelta(seconds=time_interval)
    query = {"active": "True", "last_modified_timestamp": {"$gte": cutoff}}

    workers = candidate_operations.get_candidates(params=query)

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
