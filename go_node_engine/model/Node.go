package model

import (
	"fmt"
	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	psnet "github.com/shirou/gopsutil/net"
	"go_node_engine/logger"
	"net"
	"os"
	"strconv"
	"sync"
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
	GpuDriver      string            `json:"gpu_driver"`
	GpuUsage       float64           `json:"gpu_usage"`
	GpuCores       int               `json:"gpu_cores"`
	GpuTemp        float64           `json:"gpu_temp"`
	GpuMemUsage    float64           `json:"gpu_mem_used"`
	GpuTotMem      float64           `json:"gpu_tot_mem"`
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
	err := nvml.Init()
	if err != nil {
		n.GpuDriver = "None"
		n.GpuTemp = 0
		n.GpuUsage = 0
		n.GpuMemUsage = 0
		n.GpuTotMem = 0
		n.GpuCores = 0
		logger.ErrorLogger().Printf("Unable to set GPU Info: %v", err)
	}
	defer nvml.Shutdown()

	n.GpuDriver = getGpuDriver()
	n.GpuTotMem = getTotGpuMem()
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

func getSupportedTechnologyList() []string {
	return []string{CONTAINER_RUNTIME}
}

func getGpuDriver() string {
	version, err := nvml.GetDriverVersion()
	if err != nil {
		return ""
	}

	return version
}

func getGpuMemUsage() float64 {

	count, err := nvml.GetDeviceCount()
	if err != nil {
		return 0.0
	}

	totMem := 0.0
	for i := uint(0); i < count; i++ {
		device, err := nvml.NewDevice(i)

		status, err := device.Status()
		if err != nil {
			return 0.0
		}
		totMem += float64(*status.Memory.Global.Used) * 100 / float64(*status.Memory.Global.Used+*status.Memory.Global.Free)
	}
	return totMem / float64(count)
}

func getGpuCores() int {
	count, err := nvml.GetDeviceCount()
	if err != nil {
		return 0.0
	}

	totCores := 0
	for i := uint(0); i < count; i++ {
		device, err := nvml.NewDevice(i)

		status, err := device.Status()
		if err != nil {
			return 0.0
		}
		totCores += int(*status.Clocks.Cores)

	}
	return totCores
}

func getGpuUsage() float64 {
	count, err := nvml.GetDeviceCount()
	if err != nil {
		return 0.0
	}

	totUage := 0.0
	for i := uint(0); i < count; i++ {
		device, err := nvml.NewDevice(i)

		status, err := device.Status()
		if err != nil {
			return 0.0
		}
		totUage += float64(*status.Utilization.GPU)

	}
	return totUage / float64(count)
}

func getTotGpuMem() float64 {
	count, err := nvml.GetDeviceCount()
	if err != nil {
		return 0.0
	}

	totMem := 0.0
	for i := uint(0); i < count; i++ {
		device, err := nvml.NewDevice(i)

		status, err := device.Status()
		if err != nil {
			return 0.0
		}
		totMem += float64(*status.Memory.Global.Free + *status.Memory.Global.Used)

	}
	return totMem
}

func getGpuTemp() float64 {
	count, err := nvml.GetDeviceCount()
	if err != nil {
		return 0.0
	}

	totTemp := 0.0
	for i := uint(0); i < count; i++ {
		device, err := nvml.NewDevice(i)

		status, err := device.Status()
		if err != nil {
			return 0.0
		}
		totTemp += float64(*status.Temperature)

	}
	return totTemp / float64(count)
}
