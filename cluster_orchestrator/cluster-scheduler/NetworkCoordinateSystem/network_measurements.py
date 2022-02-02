import ipaddress
import socket
import platform
from requests import get
import io
import os
import sys
from itertools import islice
import subprocess
import re

def get_ip_info():
    ip = socket.gethostbyname(socket.gethostname())
    if ipaddress.ip_address(ip).is_private:
        public_ip = get('https://api.ipify.org').text
        router_rtt = ping(public_ip)
        private_ip = ip
    else:
        public_ip = ip
        private_ip = None
        router_rtt = None

    return public_ip, private_ip, router_rtt


def ping(target_ip):
    # Parameter for number of packets differs between the operating systems
    param = "-n" if platform.system().lower() == "windows" else "-c"
    command = ["ping", param, "3", target_ip]
    print(f"Execute command: {command}")
    response = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE).communicate()
    regex_pattern = "rtt min/avg/max/mdev = (\d+.\d+)/(\d+.\d+)/(\d+.\d+)/(\d+.\d+)"
    # times = min,avg,max,mdev
    times = re.findall(regex_pattern, str(response))[0]
    avg_rtt = times[1]

    return avg_rtt

def parallel_ping(target_ips):
    ON_POSIX = 'posix' in sys.builtin_module_names
    # Create a pipe to get data
    input_fd, output_fd = os.pipe()
    # start several subprocesses
    processes = [subprocess.Popen(['ping', '-c', '3', ip], stdout=output_fd, close_fds=ON_POSIX) for ip in target_ips]
    os.close(output_fd)
    statistics = {}
    ip_pattern = "\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}"
    rtt_pattern = "rtt min/avg/max/mdev = (\d+.\d+)/(\d+.\d+)/(\d+.\d+)/(\d+.\d+)"

    with io.open(input_fd, 'r', buffering=1) as file:
        for line in file:
            if 'ping statistics' in line:
                # Find target ip
                ip_match = re.search(ip_pattern, line)
                # Find RTTs
                statistic = ''.join(islice(file, 2))
                statistic_match = re.findall(rtt_pattern, statistic)
                if len(statistic_match) != 0 and ip_match is not None:
                    ip = ip_match[0]
                    stat = statistic_match[0]
                    min_rtt = float(stat[0])
                    avg_rtt = float(stat[1])
                    statistics[ip] = min_rtt

    for p in processes:
        p.wait()

    return statistics