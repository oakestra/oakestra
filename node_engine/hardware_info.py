import psutil
import platform
from datetime import datetime
import socket

import GPUtil
from tabulate import tabulate



class HardwareInfo:

    def __init__(self):
        print('init hw class')
        self.host = socket.gethostname()
        self.ip = socket.gethostbyname(self.host)
        print(self.ip)
        self.system_info()
        self.cpu_info()
        self.memory_info()
        self.disk_info()
        self.network_info()
        self.gpu_info()
    
    
    @staticmethod
    def get_size(bytes, suffix="B"):
        """
        Scale bytes to its proper format
        e.g:
            1253656 => '1.20MB'
            1253656678 => '1.17GB'
        """
        factor = 1024
        for unit in ["", "K", "M", "G", "T", "P"]:
            if bytes < factor:
                return f"{bytes:.2f}{unit}{suffix}"
            bytes /= factor


    def system_info(self):
        print("="*40, "System Information", "="*40)
        self.uname = platform.uname()
        print(f"System: {self.uname.system}")
        print(f"Node Name: {self.uname.node}")
        print(f"Release: {self.uname.release}")
        print(f"Version: {self.uname.version}")
        print(f"Machine: {self.uname.machine}")
        print(f"Processor: {self.uname.processor}")

        # Boot Time
        print("="*40, "Boot Time", "="*40)
        self.boot_time_timestamp = psutil.boot_time()
        bt = datetime.fromtimestamp(self.boot_time_timestamp)
        print(f"Boot Time: {bt.year}/{bt.month}/{bt.day} {bt.hour}:{bt.minute}:{bt.second}")


    def cpu_info(self):
        # let's print CPU information
        print("="*40, "CPU Info", "="*40)

        # number of cores
        self.cpu_count_physical = psutil.cpu_count(logical=False)
        self.cpu_count_total = psutil.cpu_count(logical=True)
        print("Physical cores:", self.cpu_count_physical)
        print("Total cores:", self.cpu_count_total)
        # CPU frequencies
        self.cpufreq = psutil.cpu_freq()
        print(f"Max Frequency: {self.cpufreq.max:.2f}Mhz")
        print(f"Min Frequency: {self.cpufreq.min:.2f}Mhz")
        print(f"Current Frequency: {self.cpufreq.current:.2f}Mhz")
        # CPU usage
        print("CPU Usage Per Core:")
        for i, percentage in enumerate(psutil.cpu_percent(percpu=True, interval=1)):
            print(f"Core {i}: {percentage}%")
        print(f"Total CPU Usage: {psutil.cpu_percent()}%")


    def memory_info(self):
        # Memory Information
        print("="*40, "Memory Information", "="*40)
        # get the memory details
        self.svmem = psutil.virtual_memory()
        print(f"Total: {self.get_size(self.svmem.total)}")
        print(f"Available: {self.get_size(self.svmem.available)}")
        print(f"Used: {self.get_size(self.svmem.used)}")
        print(f"Percentage: {self.svmem.percent}%")
        print("="*20, "SWAP", "="*20)
        # get the swap memory details (if exists)
        self.swap = psutil.swap_memory()
        print(f"Total: {self.get_size(self.swap.total)}")
        print(f"Free: {self.get_size(self.swap.free)}")
        print(f"Used: {self.get_size(self.swap.used)}")
        print(f"Percentage: {self.swap.percent}%")


    def disk_info(self):
        # Disk Information
        print("="*40, "Disk Information", "="*40)
        print("Partitions and Usage:")
        # get all disk partitions
        self.partitions = psutil.disk_partitions()
        for partition in self.partitions:
            print(f"=== Device: {partition.device} ===")
            print(f"  Mountpoint: {partition.mountpoint}")
            print(f"  File system type: {partition.fstype}")
            try:
                partition_usage = psutil.disk_usage(partition.mountpoint)
            except PermissionError:
                # this can be catched due to the disk that
                # isn't ready
                continue
            print(f"  Total Size: {self.get_size(partition_usage.total)}")
            print(f"  Used: {self.get_size(partition_usage.used)}")
            print(f"  Free: {self.get_size(partition_usage.free)}")
            print(f"  Percentage: {partition_usage.percent}%")
        # get IO statistics since boot
        self.disk_io = psutil.disk_io_counters()
        print(f"Total read: {self.get_size(self.disk_io.read_bytes)}")
        print(f"Total write: {self.get_size(self.disk_io.write_bytes)}")


    def network_info(self):
        # Network information
        print("="*40, "Network Information", "="*40)
        # get all network interfaces (virtual and physical)
        self.if_addrs = psutil.net_if_addrs()
        for interface_name, interface_addresses in self.if_addrs.items():
            for address in interface_addresses:
                print(f"=== Interface: {interface_name} ===")
                if str(address.family) == 'AddressFamily.AF_INET':
                    print(f"  IP Address: {address.address}")
                    print(f"  Netmask: {address.netmask}")
                    print(f"  Broadcast IP: {address.broadcast}")
                elif str(address.family) == 'AddressFamily.AF_PACKET':
                    print(f"  MAC Address: {address.address}")
                    print(f"  Netmask: {address.netmask}")
                    print(f"  Broadcast MAC: {address.broadcast}")
        # get IO statistics since boot
        self.net_io = psutil.net_io_counters()
        print(f"Total Bytes Sent: {self.get_size(self.net_io.bytes_sent)}")
        print(f"Total Bytes Received: {self.get_size(self.net_io.bytes_recv)}")


    def gpu_info(self):
        print("="*40, "GPU Details", "="*40)
        self.gpus = GPUtil.getGPUs()
        self.list_gpus = []
        for gpu in self.gpus:
            # get the GPU id
            gpu_id = gpu.id
            # name of GPU
            gpu_name = gpu.name
            # get % percentage of GPU usage of that GPU
            gpu_load = f"{gpu.load*100}%"
            # get free memory in MB format
            gpu_free_memory = f"{gpu.memoryFree}MB"
            # get used memory
            gpu_used_memory = f"{gpu.memoryUsed}MB"
            # get total memory
            gpu_total_memory = f"{gpu.memoryTotal}MB"
            # get GPU temperature in Celsius
            gpu_temperature = f"{gpu.temperature} °C"
            gpu_uuid = gpu.uuid
            self.list_gpus.append((
                gpu_id, gpu_name, gpu_load, gpu_free_memory, gpu_used_memory,
                gpu_total_memory, gpu_temperature, gpu_uuid
            ))

        print(tabulate(self.list_gpus, headers=("id", "name", "load", "free memory", "used memory", "total memory",
                                        "temperature", "uuid")))