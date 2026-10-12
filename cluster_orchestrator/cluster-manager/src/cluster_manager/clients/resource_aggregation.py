from collections import defaultdict
from collections.abc import Callable
from typing import Any, cast

from resource_abstractor_client import candidate_operations

from ..models.worker import AggregatedWorkerMetrics, WorkerMetrics


def sum_int_metric(
    workers: list[WorkerMetrics], value_fn: Callable[[WorkerMetrics], int | None]
) -> int:
    values: list[int] = [
        cast(int, value_fn(worker)) for worker in workers if value_fn(worker) is not None
    ]
    return sum(values)


def average_float_metric(
    workers: list[WorkerMetrics], value_fn: Callable[[WorkerMetrics], float | None]
) -> float:
    values: list[float] = [
        cast(float, value_fn(worker)) for worker in workers if value_fn(worker) is not None
    ]
    return 0.0 if not values else sum(values) / len(values)


def concatenate_single_list_metric(
    workers: list[WorkerMetrics], value_fn: Callable[[WorkerMetrics], Any]
) -> list[Any]:
    result = []
    for worker in workers:
        result.append(value_fn(worker))
    return result


def concatenate_multi_list_metric(
    workers: list[WorkerMetrics], value_fn: Callable[[WorkerMetrics], list[Any]]
) -> list[Any]:
    result = []
    for worker in workers:
        result.extend(value_fn(worker))
    return result


def aggregate_csi_drivers(workers: list[WorkerMetrics]) -> list[str]:
    """Merge csi_drivers from a worker into a deduplicated list of driver names.

    Node Engine advertises drivers as objects: {csi_driver_name, csi_driver_endpoint}.
    After aggregation, the cluster reports a flat list of name strings to the root.
    """
    aggregated_drivers = []

    for w in workers:
        for driver in w.csi_drivers:
            if driver.csi_driver_name is None:
                continue

            aggregated_drivers.append(driver)

    return aggregated_drivers


def aggregate_worker_metrics(workers: list[WorkerMetrics]) -> AggregatedWorkerMetrics:
    # TODO: discuss adding back non-canonical metrics
    return AggregatedWorkerMetrics(
        active_nodes=len(workers),
        vcpus=sum_int_metric(workers, lambda w: w.vcpus),
        memory=sum_int_metric(workers, lambda w: w.memory),
        vgpus=sum_int_metric(workers, lambda w: w.vgpus),
        vram=sum_int_metric(workers, lambda w: int(w.vram) if w.vram is not None else None),
        cpu_percent=average_float_metric(workers, lambda w: w.cpu_percent),
        memory_percent=average_float_metric(workers, lambda w: w.memory_percent),
        vram_percent=average_float_metric(workers, lambda w: w.vram_percent),
        gpu_percent=average_float_metric(workers, lambda w: w.gpu_usage),
        gpu_drivers=concatenate_single_list_metric(workers, lambda w: w.gpu_driver),
        virtualization=concatenate_multi_list_metric(workers, lambda w: w.virtualization),
        supported_addons=concatenate_multi_list_metric(workers, lambda w: w.supported_addons),
        csi_drivers=aggregate_csi_drivers(workers),
        aggregation_per_architecture={},
    )


def compute_aggregated_worker_metrics() -> AggregatedWorkerMetrics | None:
    worker_msgs = candidate_operations.get_candidates(active=True)
    if not worker_msgs:
        return None

    worker_metrics = [WorkerMetrics.model_validate(worker_msg) for worker_msg in worker_msgs]

    workers_metrics_by_arch: dict[str, list[WorkerMetrics]] = defaultdict(list)
    for metrics_entry in worker_metrics:
        if metrics_entry.architecture is None:
            continue

        workers_metrics_by_arch[metrics_entry.architecture].append(metrics_entry)

    aggregated_metrics: AggregatedWorkerMetrics = aggregate_worker_metrics(worker_metrics)
    aggregated_metrics.aggregation_per_architecture = {
        arch: aggregate_worker_metrics(entries) for arch, entries in workers_metrics_by_arch.items()
    }

    return aggregated_metrics
