package virtualization

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"sync"

	"time"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

type UnikernelRuntime struct {
	libVirtConnection *libvirt.Connect
	killQueue         map[string]*chan bool
	channelLock       *sync.RWMutex
}

var VMruntime = UnikernelRuntime{
	channelLock: &sync.RWMutex{},
}

var libVirtSyncOnce sync.Once

func GetLibVirtConnection() *UnikernelRuntime {
	libVirtSyncOnce.Do(func() {
		conn, err := libvirt.NewConnect("qemu:///system")
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to connect to LibVirt: %v\n", err)

		}
		VMruntime.libVirtConnection = conn
		VMruntime.killQueue = make(map[string]*chan bool)
	})
	return &VMruntime
}

func (r *UnikernelRuntime) CloseLibVirtConnection() {
	_, err := r.libVirtConnection.Close()
	if err != nil {
		logger.ErrorLogger().Fatalf("Unable to disconnect from libVirt: %v\n", err)
	}
}

func (r *UnikernelRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {

	//TODO Differ between unikernels

	killChannel := make(chan bool, 1)
	startupChannel := make(chan bool, 0)
	errorChannel := make(chan error, 0)

	r.channelLock.RLock()
	el, servicefound := r.killQueue[genTaskID(service.Sname, service.Instance)]
	r.channelLock.RUnlock()
	if !servicefound || el == nil {
		r.channelLock.Lock()
		r.killQueue[genTaskID(service.Sname, service.Instance)] = &killChannel
	} else {
		return errors.New("Service already deployed")
	}

	go r.VirtualMachineCreationRoutine(service, &killChannel, startupChannel, errorChannel, statusChangeNotificationHandler)

	if <-startupChannel != true {
		return <-errorChannel
	}

	return nil
}

func (r *UnikernelRuntime) Undeploy(service string, instance int) error {

	r.channelLock.Lock()
	defer r.channelLock.Unlock()
	hostname := genTaskID(service, instance)
	el, found := r.killQueue[hostname]
	if found && el != nil {
		logger.InfoLogger().Printf("Sending kill signal to VM with hostname: %s", hostname)
		*r.killQueue[hostname] <- true
		select {
		case res := <-*r.killQueue[hostname]:
			if res == false {
				logger.ErrorLogger().Printf("Unable to stop VM %s", hostname)
			}
		case <-time.After(5 * time.Second):
			logger.ErrorLogger().Printf("Unable to stop service %s", hostname)
		}

		delete(r.killQueue, hostname)
		return nil
	}

	return errors.New("Service not found")
}

func (r *UnikernelRuntime) VirtualMachineCreationRoutine(
	service model.Service,
	killChannel *chan bool,
	startup chan bool,
	errorchan chan error,
	statusChangeNotificationHandler func(service model.Service),
) {
	var domain *libvirt.Domain
	var domainString string
	_ = domain
	_ = domainString
	hostname := genTaskID(service.Sname, service.Instance)

	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
	}

	domainString, err := CreateXMLDomain(hostname, 1, 64, 1, "virbr0", "x86_64", "192.168.178.48")
	if err != nil {
		logger.ErrorLogger().Printf("VM configuration creation failed: %v", err)
	}

	//Create and start Virtual Machine
	domain, err = r.libVirtConnection.DomainCreateXML(domainString, 0)
	if err != nil {
		revert(err)
		return
	}

	defer func(domain *libvirt.Domain) {
		err := domain.Destroy()
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		if err != nil {
			*killChannel <- false
		} else {
			*killChannel <- true
		}
	}(domain)

	startup <- true

	select {
	case <-*killChannel:
		logger.InfoLogger().Printf("Kill channel message received for unikernel")
	}
	service.Status = model.SERVICE_DEAD
	statusChangeNotificationHandler(service)
}

func CreateXMLDomain(Hostname string, DomainID int, Memory uint, CoreCount uint, BrigeName string, Arch string, IPAddress string) (string, error) {
	var pci_index uint = 0
	CMDString := fmt.Sprintf("netdev.ipv4_addr=%s netdev.ipv4_gw_addr=192.168.122.1  netdev.ipv4_subnet_mask=255.255.255.0 --", IPAddress)
	domain := &libvirtxml.Domain{
		Name:          Hostname,
		Type:          "kvm",
		ID:            &DomainID,
		Title:         "Unikraft VM Testing",
		Memory:        &libvirtxml.DomainMemory{Value: Memory, Unit: "MiB"},
		CurrentMemory: &libvirtxml.DomainCurrentMemory{Value: Memory, Unit: "MiB"},
		VCPU:          &libvirtxml.DomainVCPU{Placement: "static", Value: CoreCount},
		CPU:           &libvirtxml.DomainCPU{Mode: "host-passthrough", Check: "none", Migratable: "on"},
		OnCrash:       "restart",
		OnPoweroff:    "destroy",
		OnReboot:      "restart",
		Devices: &libvirtxml.DomainDeviceList{
			Emulator: "/usr/bin/qemu-system-x86_64",
			Controllers: []libvirtxml.DomainController{
				{
					Type:  "pci",
					Index: &pci_index,
					Model: "pci-root",
				},
				{
					Type:  "usb",
					Model: "none",
				},
			},
			Interfaces: []libvirtxml.DomainInterface{
				{
					Source:  &libvirtxml.DomainInterfaceSource{Network: &libvirtxml.DomainInterfaceSourceNetwork{Network: "default", Bridge: BrigeName}},
					Model:   &libvirtxml.DomainInterfaceModel{Type: "virtio"},
					Address: &libvirtxml.DomainAddress{},
				},
			},
		},
		OS: &libvirtxml.DomainOS{
			Type:   &libvirtxml.DomainOSType{Type: "hvm", Arch: Arch, Machine: "pc-i440fx-6.2"},
			Kernel: "/home/patrick/unikraft/workspace/apps/app-httpreply/build/httpreply_kvm-x86_64",
			BootDevices: []libvirtxml.DomainBootDevice{
				{
					Dev: "hd",
				},
			},
			Cmdline: CMDString,
		},
	}
	return domain.Marshal()
}

func (r *UnikernelRuntime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {

	for true {
		select {
		case <-time.After(every):
			domainIDs, err := r.libVirtConnection.ListDomains()
			if err != nil {
				logger.ErrorLogger().Printf("Unable to query running domains: %v", err)
			}
			resourceList := make([]model.Resources, 0)
			for _, domainID := range domainIDs {
				domain, err := r.libVirtConnection.LookupDomainById(domainID)
				if err != nil {
					logger.ErrorLogger().Printf("Unable to get domain: %v", err)
					continue
				}
				CPUStats, err := domain.GetCPUStats(-1, 1, 0)
				if err != nil {
					logger.ErrorLogger().Printf("Unable to query domain cpu usage: %v", err)
					continue
				}
				_ = CPUStats
				Hostname, err := domain.GetName()
				//TODO
				resourceList = append(resourceList, model.Resources{
					Cpu:      fmt.Sprintf("%f", 0.0),
					Memory:   fmt.Sprintf("%f", 0.1),
					Disk:     fmt.Sprintf("%d", 0),
					Sname:    extractSnameFromTaskID(Hostname),
					Runtime:  model.UNIKERNEL_RUNTIME,
					Instance: extractInstanceNumberFromTaskID(Hostname),
				})

			}

			notifyHandler(resourceList)
		}
	}

}
