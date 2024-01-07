package model

import (
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model/gpu"
	"net"
	"os"
	"runtime"
	"strconv"
	"sync"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	psnet "github.com/shirou/gopsutil/net"
)

type RuntimeType string

const (
	CONTAINER_RUNTIME RuntimeType = "docker"
	UNIKERNEL_RUNTIME RuntimeType = "unikernel"
)

type Node struct {
	Id             string            `json:"id"`
	Host           string            `json:"host"`
	Ip             string            `json:"ip"`
	Port           string            `json:"port"`
	SystemInfo     map[string]string `json:"system_info"`
	CpuUsage       float64           `json:"cpu"`
	CpuCores       int               `json:"free_cores"`
	CpuArch        string            `json:"architecture"`
	MemoryUsed     float64           `json:"memory"`
	MemoryMB       int               `json:"memory_free_in_MB"`
	DiskInfo       map[string]string `json:"disk_info"`
	NetworkInfo    map[string]string `json:"network_info"`
	GpuDriver      string            `json:"gpu_driver"`
	GpuUsage       float64           `json:"gpu_usage"`
	GpuCores       int               `json:"gpu_cores"`
	GpuTemp        float64           `json:"gpu_temp"`
	GpuMemUsage    float64           `json:"gpu_mem_used"`
	GpuTotMem      float64           `json:"gpu_tot_mem"`
	Technology     []RuntimeType     `json:"technology"`
	Overlay        bool
	LogDirectory   string
	NetManagerPort int
}

var once sync.Once
var node Node

func GetNodeInfo() *Node {
	once.Do(func() {
		node = Node{
			Host:       getHostname(),
			SystemInfo: getSystemInfo(),
			CpuCores:   getCpuCores(),
			CpuArch:    runtime.GOARCH,
			Port:       getPort(),
			Technology: make([]RuntimeType, 0),
			Overlay:    false,
		}
	})
	node.updateDynamicInfo()
	return &node
}

func (n *Node) SetLogDirectory(dir string) {
	n.LogDirectory = dir
}

func GetDynamicInfo() Node {
	node.updateDynamicInfo()
	return Node{
		CpuUsage:    node.CpuUsage,
		CpuCores:    node.CpuCores,
		MemoryUsed:  node.MemoryUsed,
		MemoryMB:    node.MemoryMB,
		GpuDriver:   node.GpuDriver,
		GpuTemp:     node.GpuTemp,
		GpuUsage:    node.GpuUsage,
		GpuTotMem:   node.GpuTotMem,
		GpuMemUsage: node.GpuMemUsage,
	}
}

func EnableOverlay(port int) {
	node.Overlay = true
	node.NetManagerPort = port
}

func (n *Node) updateDynamicInfo() {
	// System Info
	n.CpuUsage = getAvgCpuUsage()
	n.Ip = getIp()
	n.MemoryMB = getMemoryMB()
	n.MemoryUsed = getMemoryUsage()
	n.DiskInfo = getDiskinfo()
	n.NetworkInfo = getNetworkInfo()

	// GPU Info
	n.GpuDriver = getGpuDriver()
	n.GpuTotMem = getTotGpuMemFreeMB()
	n.GpuMemUsage = getGpuMemUsage()
	n.GpuUsage = getGpuUsage()
	n.GpuCores = getGpuCores()
	n.GpuTemp = getGpuTemp()

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
	return int(mem.Available >> 20)
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

func (n *Node) AddSupportedTechnology(tech RuntimeType) {
	n.Technology = append(n.Technology, tech)
}

func (n *Node) GetSupportedTechnologyList() []RuntimeType {
	return n.Technology
}

func getGpuDriver() string {
	n, err := gpu.NvsmiDeviceCount()
	if err != nil {
		return "-"
	}

	for i := 0; i < n; i++ {
		res, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "driver_version")
		if err != nil {
			return "-"
		}
		return res
	}
	return "-"
}

func getGpuMemUsage() float64 {
	n, err := gpu.NvsmiDeviceCount()
	if err != nil || n == 0 {
		return 0
	}

	totMem := 0.0
	for i := 0; i < n; i++ {
		res, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "memory.used")
		if err != nil {
			return 0
		}
		totm := getTotGpuMem()
		if totm >= 0 {
			currmem, err := strconv.Atoi(res)
			if err != nil {
				return 0
			}
			totMem += float64(currmem) * 100 / getTotGpuMem()
		}
	}
	return totMem / float64(n)
}

func getGpuCores() int {
	n, err := gpu.NvsmiDeviceCount()
	if err != nil || n == 0 {
		return 0
	}
	return n
}

func getGpuUsage() float64 {
	n, err := gpu.NvsmiDeviceCount()
	if err != nil || n == 0 {
		return 0
	}

	totUage := 0.0
	for i := 0; i < n; i++ {
		res, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "utilization.gpu")
		if err != nil {
			return 0
		}
		gpuusage, err := strconv.Atoi(res)
		if err != nil {
			return 0
		}
		totUage += float64(gpuusage)
	}
	return totUage / float64(n)
}

func getTotGpuMem() float64 {
	n, err := gpu.NvsmiDeviceCount()
	if err != nil || n == 0 {
		return 0
	}

	totMem := 0.0
	for i := 0; i < n; i++ {
		res, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "memory.total")
		if err != nil {
			return 0
		}
		gpuMem, err := strconv.Atoi(res)
		if err != nil {
			return 0
		}
		totMem += float64(gpuMem)
	}
	return totMem
}

func getTotGpuMemFreeMB() float64 {
	n, err := gpu.NvsmiDeviceCount()
	if err != nil || n == 0 {
		return 0
	}

	totMem := 0.0
	for i := 0; i < n; i++ {
		res, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "memory.free")
		if err != nil {
			return 0
		}
		gpuMem, err := strconv.Atoi(res)
		if err != nil {
			return 0
		}
		totMem += float64(gpuMem)
	}
	return totMem
}

func getGpuTemp() float64 {
	n, err := gpu.NvsmiDeviceCount()
	if err != nil || n == 0 {
		return 0
	}

	totTemp := 0.0
	for i := 0; i < n; i++ {
		res, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "temperature.gpu")
		if err != nil {
			return 0
		}
		currTemp, err := strconv.Atoi(res)
		if err != nil {
			return 0
		}
		totTemp += float64(currTemp)
	}
	return totTemp / float64(n)
}
