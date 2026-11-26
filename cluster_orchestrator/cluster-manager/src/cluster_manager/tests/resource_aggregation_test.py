import json
from dataclasses import asdict

import pytest

from ..clients.resource_aggregation import (
    aggregate_worker_metrics,
    compute_aggregated_worker_metrics,
)
from ..models.worker import WorkerMetrics


def test_gpu_vram_free_is_elementwise_max_of_sorted_lists():
    workers = [
        WorkerMetrics(gpu_vram_free=[4000.0, 24000.0]),
        WorkerMetrics(gpu_vram_free=[8000.0]),
    ]
    assert aggregate_worker_metrics(workers).gpu_vram_free == [24000.0, 4000.0]


def test_gpu_vram_free_does_not_sum_single_gpu_nodes():
    workers = [WorkerMetrics(gpu_vram_free=[8000.0]), WorkerMetrics(gpu_vram_free=[8000.0])]
    assert aggregate_worker_metrics(workers).gpu_vram_free == [8000.0]


def test_gpu_vram_free_legacy_fallback_splits_total_vram():
    workers = [WorkerMetrics(vgpus=2, vram=16000)]
    assert aggregate_worker_metrics(workers).gpu_vram_free == [8000.0, 8000.0]


def test_gpu_vram_free_empty_without_gpus():
    assert aggregate_worker_metrics([WorkerMetrics(vgpus=0)]).gpu_vram_free == []
    assert aggregate_worker_metrics([]).gpu_vram_free == []


def test_gpu_drivers_from_gpu_driver_deduplicated():
    workers = [
        WorkerMetrics(gpu_driver="550.1"),
        WorkerMetrics(gpu_driver="550.1"),
        WorkerMetrics(gpu_driver="-"),
        WorkerMetrics(gpu_driver=""),
        WorkerMetrics(gpu_driver="535.2"),
        WorkerMetrics(),
    ]
    assert aggregate_worker_metrics(workers).gpu_drivers == ["550.1", "535.2"]


def test_gpu_drivers_empty_default():
    assert aggregate_worker_metrics([WorkerMetrics()]).gpu_drivers == []
    assert aggregate_worker_metrics([]).gpu_drivers == []


def test_gpu_vram_free_preserves_reported_values_and_worker_order():
    workers = [
        WorkerMetrics(vgpus=2, vram=48000, gpu_vram_free=[4000.5, 24000.0]),
        WorkerMetrics(vgpus=2, vram=32000, gpu_vram_free=[8000.25, 12000.0]),
    ]
    for ordering in (workers, list(reversed(workers))):
        assert aggregate_worker_metrics(ordering).gpu_vram_free == [24000.0, 8000.25]
    assert workers[0].gpu_vram_free == [4000.5, 24000.0]


def test_gpu_vram_free_mixes_legacy_and_current_workers():
    workers = [
        WorkerMetrics(vgpus=2, vram=16000),
        WorkerMetrics(gpu_vram_free=[24000.0]),
    ]
    assert aggregate_worker_metrics(workers).gpu_vram_free == [24000.0, 8000.0]


def test_gpu_vram_free_explicit_empty_list_does_not_use_legacy_fallback():
    worker = WorkerMetrics(vgpus=2, vram=16000, gpu_vram_free=[])
    assert aggregate_worker_metrics([worker]).gpu_vram_free == []


def test_compute_aggregated_worker_metrics_serializes_gpu_fields_per_architecture(candidates_mock):
    worker_msgs = [
        {
            "architecture": "amd64",
            "gpu_driver": "550.1",
            "gpu_percent": 20.0,
            "gpu_vram_free": [4000, 24000],
        },
        {
            "architecture": "arm64",
            "gpu_driver": "535.2",
            "gpu_percent": 60.0,
            "vgpus": 2,
            "vram": 16000,
        },
    ]
    candidates_mock.get_candidates.return_value = worker_msgs
    metrics = compute_aggregated_worker_metrics()

    candidates_mock.get_candidates.assert_called_once_with(active=True, resources="gpu_driver")
    assert metrics is not None
    data = asdict(metrics)
    assert data["gpu_vram_free"] == [24000.0, 8000.0]
    assert data["gpu_drivers"] == ["550.1", "535.2"]
    assert data["gpu_percent"] == 40.0
    by_arch = data["aggregation_per_architecture"]
    assert by_arch["amd64"]["gpu_vram_free"] == [24000.0, 4000.0]
    assert by_arch["amd64"]["gpu_drivers"] == ["550.1"]
    assert by_arch["amd64"]["gpu_percent"] == 20.0
    assert by_arch["arm64"]["gpu_vram_free"] == [8000.0, 8000.0]
    assert by_arch["arm64"]["gpu_drivers"] == ["535.2"]
    assert by_arch["arm64"]["gpu_percent"] == 60.0


def test_csi_drivers_are_unique_names_in_serializable_cluster_report(candidates_mock):
    candidates_mock.get_candidates.return_value = [
        {
            "architecture": "amd64",
            "csi_drivers": [
                {"csi_driver_name": "hostpath", "csi_driver_endpoint": "/run/csi/hostpath.sock"},
                {"csi_driver_name": "hostpath"},
                {"csi_driver_name": ""},
                {},
            ],
        },
        {
            "architecture": "arm64",
            "csi_drivers": [
                {"csi_driver_name": "hostpath", "csi_driver_endpoint": "/run/csi/hostpath.sock"},
                {"csi_driver_name": "nfs", "csi_driver_endpoint": "/run/csi/nfs.sock"},
            ],
        },
    ]
    metrics = compute_aggregated_worker_metrics()

    assert metrics is not None
    data = json.loads(json.dumps(asdict(metrics)))
    assert data["csi_drivers"] == ["hostpath", "nfs"]
    assert data["aggregation_per_architecture"]["amd64"]["csi_drivers"] == ["hostpath"]
    assert data["aggregation_per_architecture"]["arm64"]["csi_drivers"] == ["hostpath", "nfs"]


@pytest.mark.parametrize("worker_msgs", [None, []])
def test_compute_aggregated_worker_metrics_without_workers(worker_msgs, candidates_mock):
    candidates_mock.get_candidates.return_value = worker_msgs
    assert compute_aggregated_worker_metrics() is None
