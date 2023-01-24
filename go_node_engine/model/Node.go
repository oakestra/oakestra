package model

import (
	"fmt"
	"go_node_engine/logger"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/jaypipes/ghw"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	psnet "github.com/shirou/gopsutil/net"
)

const (
	CONTAINER_RUNTIME = "docker"
	UNIKERNEL_RUNTIME = "unikernel"
)

type Node struct {
	Id             string            `json:"id"`
	Host           string            `json:"host"`
	Ip             string            `json:"ip"`
	Port           string            `json:"port"`
	SystemInfo     map[string]string `json:"system_info"`
	CpuUsage       float64           `json:"cpu"`
	CpuCores       int               `json:"free_cores"`
	MemoryUsed     float64           `json:"memory"`
	MemoryMB       int               `json:"memory_free_in_MB"`
	DiskInfo       map[string]string `json:"disk_info"`
	NetworkInfo    map[string]string `json:"network_info"`
	GpuInfo        map[string]string `json:"gpu_info"`
	Technology     []string          `json:"technology"`
	Overlay        bool
	NetManagerPort int
}

var once sync.Once
var node Node

func GetNodeInfo() Node {
	once.Do(func() {
		node = Node{
			Host:       getHostname(),
			SystemInfo: getSystemInfo(),
			CpuCores:   getCpuCores(),
			Port:       getPort(),
			Technology: getSupportedTechnologyList(),
			Overlay:    false,
		}
	})
	node.updateDynamicInfo()
	return node
}

func GetDynamicInfo() Node {
	node.updateDynamicInfo()
	return Node{
		CpuUsage:   node.CpuUsage,
		CpuCores:   node.CpuCores,
		MemoryUsed: node.MemoryUsed,
		MemoryMB:   node.MemoryMB,
	}
}

func EnableOverlay(port int) {
	node.Overlay = true
	node.NetManagerPort = port
}

func (n *Node) updateDynamicInfo() {
	n.CpuUsage = getAvgCpuUsage()
	n.Ip = getIp()
	n.MemoryMB = getMemoryMB()
	n.MemoryUsed = getMemoryUsage()
	n.DiskInfo = getDiskinfo()
	n.NetworkInfo = getNetworkInfo()
	n.GpuInfo = getGpuInfo()
}

func SetNodeId(id string) {
	GetNodeInfo()
	node.Id = id
}

func getIp() string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addresses {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
		logger.ErrorLogger().Fatal("Unable to get Node hostname")
	}
	return hostname
}

func getSystemInfo() map[string]string {
	hostinfo, err := host.Info()
	if err != nil {
		logger.ErrorLogger().Printf("Error: %s", err.Error())
		return make(map[string]string, 0)
	}
	sysInfo := make(map[string]string)
	sysInfo["kernel_version"] = hostinfo.KernelVersion
	sysInfo["architecture"] = hostinfo.KernelArch
	sysInfo["os_version"] = hostinfo.OS
	sysInfo["uptime"] = strconv.Itoa(int(hostinfo.Uptime))
	sysInfo["full_stats"] = hostinfo.String()

	return sysInfo
}

func getCpuCores() int {
	cpu, err := cpu.Counts(true)
	if err != nil {
		logger.ErrorLogger().Printf("Error: %s", err.Error())
		return 0
	}
	return cpu
}

func getAvgCpuUsage() float64 {
	avg, err := load.Avg()
	if err != nil {
		return 100
	}
	return avg.Load5
}

func getMemoryMB() int {
	mem, err := mem.VirtualMemory()
	if err != nil {
		logger.ErrorLogger().Printf("Error: %s", err.Error())
		return 0
	}
	return int(mem.Free >> 20)
}

func getMemoryUsage() float64 {
	mem, err := mem.VirtualMemory()
	if err != nil {
		logger.ErrorLogger().Printf("Error: %s", err.Error())
		return 100
	}
	return mem.UsedPercent
}

func getDiskinfo() map[string]string {
	diskUsageStats, err := disk.Usage("/")
	diskInfoMap := make(map[string]string, 0)
	usage := "100"
	if err == nil {
		usage = strconv.Itoa(int(diskUsageStats.UsedPercent))
	}
	diskInfoMap["/"] = usage
	partitionsStats, err := disk.Partitions(true)
	if err == nil {
		for i, partition := range partitionsStats {
			diskInfoMap[fmt.Sprintf("partition_%d", i)] = partition.String()
		}
	}
	return diskInfoMap
}

func getGpuInfo() map[string]string {
	gpu, err := ghw.GPU()
	gpuInfoMap := make(map[string]string)
	if err != nil {
		fmt.Printf("Error %v", err)
		return gpuInfoMap
	}
	for i, card := range gpu.GraphicsCards {
		gpuInfoMap[fmt.Sprintf("gpu_%d", i)] = card.String()
	}
	return gpuInfoMap
}

func getNetworkInfo() map[string]string {
	netInfoMap := make(map[string]string)
	interfaces, err := psnet.Interfaces()
	if err == nil {
		for i, ifce := range interfaces {
			netInfoMap[fmt.Sprintf("interface_%d", i)] = ifce.String()
		}
	}
	return netInfoMap
}

func getPort() string {
	port := os.Getenv("MY_PORT")
	if port == "" {
		port = "3000"
	}
	return port
}

func getSupportedTechnologyList() []string {
	return []string{CONTAINER_RUNTIME}
}
