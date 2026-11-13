from clients.resource_aggregation import aggregate_workers


def test_gpu_vram_free_is_elementwise_max_of_sorted_lists():
    workers = [
        {"gpu_vram_free": [4000.0, 24000.0]},
        {"gpu_vram_free": [8000.0]},
    ]
    assert aggregate_workers(workers)["gpu_vram_free"] == [24000.0, 4000.0]


def test_gpu_vram_free_does_not_sum_single_gpu_nodes():
    workers = [{"gpu_vram_free": [8000.0]}, {"gpu_vram_free": [8000.0]}]
    assert aggregate_workers(workers)["gpu_vram_free"] == [8000.0]


def test_gpu_vram_free_legacy_fallback_splits_total_vram():
    workers = [{"vgpus": 2, "vram": 16000}]
    assert aggregate_workers(workers)["gpu_vram_free"] == [8000.0, 8000.0]


def test_gpu_vram_free_empty_without_gpus():
    assert aggregate_workers([{"vgpus": 0}])["gpu_vram_free"] == []
    assert aggregate_workers([])["gpu_vram_free"] == []


def test_gpu_drivers_from_gpu_driver_deduplicated():
    workers = [
        {"gpu_driver": "550.1"},
        {"gpu_driver": "550.1"},
        {"gpu_driver": "-"},
        {"gpu_driver": ""},
        {"gpu_driver": "535.2"},
        {},
    ]
    assert aggregate_workers(workers)["gpu_drivers"] == ["550.1", "535.2"]


def test_gpu_drivers_empty_default():
    assert aggregate_workers([{}])["gpu_drivers"] == []
