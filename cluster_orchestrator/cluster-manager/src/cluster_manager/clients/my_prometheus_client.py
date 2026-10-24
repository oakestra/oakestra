import logging
from typing import Any

from prometheus_client import Gauge

metrics = {}
cluster_id: str | None = None
logger: logging.Logger | None = None


def add_or_set_metric(name: Any, value: Any) -> None:
    metrics_name = "_gauge_" + str(name) + "_" + str(cluster_id)
    if type(value) is not list and type(value) is not dict and value is not None:
        try:
            if metrics_name in metrics:
                metrics[metrics_name].set(value)
            else:
                metrics[metrics_name] = Gauge(metrics_name, "")
        except Exception as e:
            if not logger:
                raise RuntimeError("add_or_set_metric was used before logger was initialized")
            logger.error("Unable to set metric " + metrics_name + " to " + str(value))
            logger.error(e)


def prometheus_init_gauge_metrics(assigned_cluster_id: str, app_logger: logging.Logger) -> None:
    global cluster_id, logger
    logger = app_logger
    cluster_id = assigned_cluster_id
    print("prometheus gauge metrics initialized.")


def prometheus_set_metrics(data: dict[Any, Any]) -> None:
    for metric_name, metric_value in data.items():
        add_or_set_metric(metric_name, metric_value)
