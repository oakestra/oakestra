from prometheus_client import Gauge

metrics = {}
jobs = {}
cluster_id = None
logger = None


def add_or_set_metric(name, value):
    global metrics, logger
    metrics_name = "_gauge_" + str(name) + "_" + str(cluster_id)
    if type(value) is not list and type(value) is not dict and value is not None:
        try:
            if metrics_name in metrics:
                metrics[metrics_name].set(value)
            else:
                metrics[metrics_name] = Gauge(metrics_name, "")
        except Exception as e:
            logger.error("Unable to set metric " + metrics_name + " to " + str(value))
            logger.error(e)


def prometheus_init_gauge_metrics(my_id, app_logger):
    global cluster_id, logger
    logger = app_logger
    cluster_id = my_id
    print("prometheus gauge metrics initialized.")


def prometheus_set_metrics(data):
    for metric_name, metric_value in data.items():
        add_or_set_metric(metric_name, metric_value)
