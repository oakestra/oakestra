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
    # s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    # s.connect(("8.8.8.8", 80))
    # ip = s.getsockname()[0]
    # s.close()
    # TODO: can the IP address obtained via socket even be a public ip?
    if ipaddress.ip_address(ip).is_private:
        # TODO: check how to use netmanager to contact nodes in another network
        #public_ip = get('https://api.ipify.org').text
        resp = get('https://api4.ipify.org?format=json').text
        import ast
        public_ip = ast.literal_eval(resp)['ip']
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
    # TODO: how many packets should we send?
    command = ["ping", param, "3", target_ip]
    print(f"PING {target_ip}")
    response = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE).communicate()
    regex_pattern = "rtt min/avg/max/mdev = (\d+.\d+)/(\d+.\d+)/(\d+.\d+)/(\d+.\d+)"
    # times = min,avg,max,mdev
    # TODO: use min rtt. for some reason first ping in docker is twice as expected latency
    times = re.findall(regex_pattern, str(response))[0]
    avg_rtt = times[0]

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
    print(f"TARGET IPS: {target_ips}")
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


def get_netem_delay(is_netem_configured):
    if is_netem_configured:
        import subprocess
        command = ['tc', 'qdisc']
        response = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE).communicate()
        resp = str(response[0])
        delay_idx = resp.index('delay')
        ms_idx = resp.index('ms')
        netem_delay = resp[delay_idx + 6 : ms_idx]
        return netem_delay
    else:
        return "0.0"