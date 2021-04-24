import psutil


def get_cpu_memory():
    # gives a single float value
    cpu_count_total = psutil.cpu_count(logical=True)
    cpu_used = psutil.cpu_percent()
    mem_used = psutil.virtual_memory().percent

    free_cores = cpu_count_total * (1 - (cpu_used/ 100))
    free_memory_in_MB = float(f"{psutil.virtual_memory().available/1024/1024:.2f}")

    print("cpu_used: {0:5f}".format(cpu_used))
    print("free_cores: {0:5f}".format(free_cores))
    print("memory_used: {0}".format(mem_used))
    print("memory_free_in_MB: {0}MB".format(free_memory_in_MB))
    return cpu_used, free_cores, mem_used, free_memory_in_MB


def get_memory():
    
    # you can have the percentage of used RAM
    mem_used = psutil.virtual_memory().percent

    # you can calculate percentage of available memory
    # mem_available = psutil.virtual_memory().available * 100 / psutil.virtual_memory().total
    
    print('memory: {0:5f}'.format(mem_used))
    return mem_used
