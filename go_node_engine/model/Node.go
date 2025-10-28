package model

import (
	"fmt"
	"go_node_engine/config"
	"go_node_engine/logger"
	"go_node_engine/model/gpu"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	psnet "github.com/shirou/gopsutil/net"
)

type RuntimeType = config.RuntimeType

// AddonType is the type of addon that the node supports
var (
	CONTAINER_RUNTIME config.RuntimeType = config.CONTAINER_RUNTIME
	UNIKERNEL_RUNTIME config.RuntimeType = config.UNIKERNEL_RUNTIME
)

const SlowUpdateFactor = 60 // For updating certain Node parameters at a lower frequency
var SlowUpdateCounter = 0

type AddonType string

const (
	IMAGE_BUILDER AddonType = "image-builder"
	FLOPS_LEARNER AddonType = "FLOps-learner"
)

// Node is the struct that describes the node
type Node struct {
	Id              string               `json:"id"`
	Host            string               `json:"host"`
	Ip              string               `json:"ip"`
	Port            string               `json:"port"`
	SystemInfo      map[string]string    `json:"system_info"`
	CpuUsage        float64              `json:"cpu_percent"`
	CpuCores        int                  `json:"vcpus"`
	CpuArch         string               `json:"architecture"`
	MemoryUsed      float64              `json:"memory_percent"`
	MemoryMB        int                  `json:"memory"`
	DiskInfo        map[string]string    `json:"disk_info"`
	NetworkInfo     map[string]string    `json:"network_info"`
	GpuDriver       string               `json:"gpu_driver"`
	GpuUsage        float64              `json:"gpu_usage"`
	GpuCores        int                  `json:"vgpus"`
	GpuTemp         float64              `json:"gpu_temp"`
	GpuMemUsage     float64              `json:"vram_percent"`
	GpuTotMem       float64              `json:"vram"`
	Technology      []config.RuntimeType `json:"virtualization"`
	SupportedAddons []AddonType          `json:"supported_addons"`
	CarbonIntensity float64              `json:"carbon_intensity"`
	Overlay         bool
	OverlaySocket   string
	LogDirectory    string
	NetManagerPort  int
	ClusterAddress  string
}

var once sync.Once
var node Node

// GetNodeInfo returns the node information
func GetNodeInfo() *Node {
	once.Do(func() {
		node = Node{
			Host:            getHostname(),
			SystemInfo:      getSystemInfo(),
			CpuCores:        getCpuCores(),
			CpuArch:         runtime.GOARCH,
			Port:            getPort(),
			Technology:      make([]config.RuntimeType, 0),
			SupportedAddons: make([]AddonType, 0),
			Overlay:         false,
			OverlaySocket:   "/etc/netmanager/netmanager.sock",
		}
	})
	node.updateDynamicInfo()
	return &node
}

// SetLogDirectory sets the directory where the logs will be stored
func (n *Node) SetLogDirectory(dir string) {
	n.LogDirectory = dir
}

func (n *Node) SetClusterAddress(addr string) {
	n.ClusterAddress = addr
}

func (n *Node) SetOverlaySocket(socket string) {
	n.OverlaySocket = socket
}

// GetDynamicInfo returns the dynamic information of the node (CPU, Memory, GPU usage etc.)
func GetDynamicInfo() Node {
	node.updateDynamicInfo()
	return Node{
		Ip:              node.Ip,
		CpuUsage:        node.CpuUsage,
		CpuCores:        node.CpuCores,
		MemoryUsed:      node.MemoryUsed,
		MemoryMB:        node.MemoryMB,
		GpuDriver:       node.GpuDriver,
		GpuTemp:         node.GpuTemp,
		GpuUsage:        node.GpuUsage,
		GpuTotMem:       node.GpuTotMem,
		GpuMemUsage:     node.GpuMemUsage,
		CarbonIntensity: node.CarbonIntensity,
	}
}

// EnableOverlay enables the overlay network, setting the port
func EnableOverlay() {
	node.Overlay = true
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

	// Custom Info
	n.CarbonIntensity = getCarbonIntensity()
}

// SetNodeId sets the node id
func SetNodeId(id string) {
	GetNodeInfo()
	node.Id = id
}

func getCarbonIntensity() float64 {
	return 42.0
}

func getIp() string {
	conf, err := config.GetConfFileManager().Get()
	if err != nil {
		logger.ErrorLogger().Fatal(err)
	}
	if conf.PublicIp {

		// Only get public IP every nth update cycle to prevent API overload
		if SlowUpdateCounter != 0 {
			SlowUpdateCounter = (SlowUpdateCounter + 1) % SlowUpdateFactor
			return node.Ip
		}
		SlowUpdateCounter = (SlowUpdateCounter + 1) % SlowUpdateFactor

		req, err := http.Get("https://ifconfig.co")
		if err != nil {
			logger.ErrorLogger().Printf("%v", err.Error())
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				logger.ErrorLogger().Printf("%v", err.Error())
			}
		}(req.Body)

		body, err := io.ReadAll(req.Body)
		if err != nil {
			return err.Error()
		}

		return string(body[:len(body)-1])
	}

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
	avg, err := cpu.Percent(0, false)
	if err != nil {
		return 100
	}
	if len(avg) == 0 {
		return 100
	}
	return avg[0]
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

// AddSupportedTechnology adds a supported technology to the node
func (n *Node) AddSupportedTechnology(tech config.RuntimeType) {
	n.Technology = append(n.Technology, tech)
}

// GetSupportedTechnologyList returns the list of supported technologies
func (n *Node) GetSupportedTechnologyList() []config.RuntimeType {
	return n.Technology
}

// AddSupportedAddons adds a supported addon to the node
func (n *Node) AddSupportedAddons(ext AddonType) {
	n.SupportedAddons = append(n.SupportedAddons, ext)
}

// GetSupportedAddonsList returns the list of supported addons
func (n *Node) GetSupportedAddonsList() []AddonType {
	return n.SupportedAddons
}

func getGpuDriver() string {
	n, err := gpu.NvsmiDeviceCount()
	if err != nil {
		return "-"
	}

	var queryResult string
	for i := 0; i < n; i++ {
		res, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "driver_version")
		if err != nil {
			continue
		}
		queryResult = res
	}

	if queryResult != "" {
		return "-"
	}
	return queryResult
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
