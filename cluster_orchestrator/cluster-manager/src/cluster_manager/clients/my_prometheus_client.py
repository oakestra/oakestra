from typing import Any

from oakestra_logging import get_logger
from prometheus_client import Gauge

metrics = {}
cluster_id: str | None = None
logger = get_logger(__name__)


def add_or_set_metric(name: Any, value: Any) -> None:
    metrics_name = "_gauge_" + str(name) + "_" + str(cluster_id)
    if type(value) is not list and type(value) is not dict and value is not None:
        try:
            if metrics_name in metrics:
                metrics[metrics_name].set(value)
            else:
                metrics[metrics_name] = Gauge(metrics_name, "")
        except Exception:
            logger.exception(
                "Unable to set Prometheus metric",
                event_name="prometheus.metric.update_failed",
                metric_name=metrics_name,
            )


def prometheus_init_gauge_metrics(assigned_cluster_id: str) -> None:
    global cluster_id
    cluster_id = assigned_cluster_id
    logger.info(
        "Initialized Prometheus gauge metrics",
        event_name="prometheus.metrics.initialized",
        cluster_id=assigned_cluster_id,
    )


def prometheus_set_metrics(data: dict[Any, Any]) -> None:
    for metric_name, metric_value in data.items():
        add_or_set_metric(metric_name, metric_value)
