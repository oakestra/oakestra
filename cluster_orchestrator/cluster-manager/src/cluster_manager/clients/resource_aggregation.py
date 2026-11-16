from collections import defaultdict
from collections.abc import Callable, Collection
from itertools import zip_longest
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
    workers: list[WorkerMetrics],
    value_fn: Callable[[WorkerMetrics], Any],
    *,
    skip_values: Collection[Any] = (),
    unique: bool = False,
) -> list[Any]:
    """Collect one value per worker, optionally skipping values and removing duplicates."""
    return concatenate_multi_list_metric(
        workers,
        lambda worker: [value_fn(worker)],
        skip_values=skip_values,
        unique=unique,
    )


def concatenate_multi_list_metric(
    workers: list[WorkerMetrics],
    value_fn: Callable[[WorkerMetrics], list[Any]],
    *,
    skip_values: Collection[Any] = (),
    unique: bool = False,
) -> list[Any]:
    """Flatten worker lists, preserving the order of retained values."""
    result = []
    for worker in workers:
        for value in value_fn(worker):
            if value in skip_values or (unique and value in result):
                continue
            result.append(value)
    return result


def max_float_list_metric(
    workers: list[WorkerMetrics], value_fn: Callable[[WorkerMetrics], list[float]]
) -> list[float]:
    """Take element-wise maxima, padding shorter lists with zero."""
    values_by_worker = [value_fn(worker) for worker in workers]
    return [max(values) for values in zip_longest(*values_by_worker, fillvalue=0.0)]


def _worker_gpu_vram_free(worker: WorkerMetrics) -> list[float]:
    """Return free VRAM per GPU, sorted largest first."""
    if worker.gpu_vram_free is not None:
        return sorted(worker.gpu_vram_free, reverse=True)

    # Legacy workers do not report per-GPU free VRAM; assume an even split of total VRAM.
    vgpus = worker.vgpus or 0
    if vgpus > 0:
        return [(worker.vram or 0.0) / vgpus] * vgpus
    return []


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
        gpu_drivers=concatenate_single_list_metric(
            workers, lambda w: w.gpu_driver, skip_values=(None, "", "-"), unique=True
        ),
        # Entry N-1 meets a per-GPU VRAM requirement only if one worker has N GPUs that fit.
        gpu_vram_free=max_float_list_metric(workers, _worker_gpu_vram_free),
        virtualization=concatenate_multi_list_metric(workers, lambda w: w.virtualization),
        supported_addons=concatenate_multi_list_metric(workers, lambda w: w.supported_addons),
        csi_drivers=aggregate_csi_drivers(workers),
        aggregation_per_architecture={},
    )


def compute_aggregated_worker_metrics() -> AggregatedWorkerMetrics | None:
    # The resource abstractor only projects its canonical fields unless asked for more,
    # and the per-worker "gpu_driver" is not one of them.
    worker_msgs = candidate_operations.get_candidates(active=True, resources="gpu_driver")
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
