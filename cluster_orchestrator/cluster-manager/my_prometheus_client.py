from prometheus_client import Gauge

gauge_cpu = None
gauge_memory = None


def prometheus_init_gauge_metrics(my_id):
    global gauge_cpu, gauge_memory
    gauge_cpu = Gauge('_gauge_cpu_' + my_id, 'Total available CPU cores Gauge')
    gauge_memory = Gauge('_gauge_memory_' + my_id, 'Total available Memory Gauge')
    print('prometheus gauge metrics initialized.')


def prometheus_set_metrics(my_id, data):
    gauge_cpu.set(data.get('cpu_cores'))
    gauge_memory.set(data.get('cumulative_memory_in_mb'))
    print('Metrics set.')
