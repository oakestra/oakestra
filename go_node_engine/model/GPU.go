package model

import (
	nvml "github.com/NVIDIA/go-nvml/pkg/nvml"
	"go_node_engine/logger"
	"sync"
)

type GPUStats struct {
	GPUUsage    uint32 `json:"gpu_usage"`
	MemoryUsage uint64 `json:"memory_usage"`
}

var getDevicesOnce sync.Once
var nvmlDevices []nvml.Device

func getDevices() []nvml.Device {
	getDevicesOnce.Do(func() {
		nvmlDevices = nil
		ret := nvml.Init()
		if ret != nvml.SUCCESS {
			logger.ErrorLogger().Printf("Unable to initialize NVML, %s", nvml.ErrorString(ret))
			return
		}
		count, ret := nvml.DeviceGetCount()
		if ret != nvml.SUCCESS {
			logger.ErrorLogger().Printf("Unable to initialize NVML, %s", nvml.ErrorString(ret))
			return
		}
		if count >= 0 {
			nvmlDevices = make([]nvml.Device, count)
			for di := 0; di < count; di++ {
				device, ret := nvml.DeviceGetHandleByIndex(di)
				if ret != nvml.SUCCESS {
					logger.ErrorLogger().Printf("Unable to get device at index %d: %v", di, nvml.ErrorString(ret))
				}
				nvmlDevices[di] = device
			}
		}
	})
	return nvmlDevices
}

func GetGPUStatistics(pid uint32) GPUStats {
	devices := getDevices()
	if devices != nil {
		for _, device := range devices {
			stats, ret := device.GetAccountingStats(pid)
			if ret == nvml.SUCCESS {
				return GPUStats{
					GPUUsage:    stats.GpuUtilization,
					MemoryUsage: stats.MaxMemoryUsage,
				}
			}
		}
	}
	return GPUStats{}
}
