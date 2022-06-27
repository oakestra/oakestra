package virtualization

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"reflect"
	"sync"

	"time"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

type UnikernelRuntime struct {
	libVirtConnection *libvirt.Connect
	killQueue         map[string]*chan bool
	channelLock       *sync.RWMutex
	network           *libvirt.Network
	subnet            string
	IPaddresses       map[int]int
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

		networks, err := VMruntime.libVirtConnection.ListAllNetworks(libvirt.CONNECT_LIST_NETWORKS_ACTIVE)
		if err != nil {
			logger.InfoLogger().Printf("Unable to list the network interfaces %v", err)
		}
		if !model.GetNodeInfo().Overlay {
			if len(networks) < 1 {
				logger.InfoLogger().Printf("No local Network for libvirt found")
			}

			for i, n := range networks {
				if i == 0 {
					name, _ := n.GetBridgeName()
					logger.InfoLogger().Printf("Using %s as default Network", name)
					test, _ := n.GetXMLDesc(0)
					var networkDescription libvirtxml.Network
					networkDescription.Unmarshal(test)
					VMruntime.subnet = networkDescription.IPs[0].Address[0 : len(networkDescription.IPs[0].Address)-1]
					VMruntime.network = &n
				} else {
					n.Free()
				}
			}
		} else {
			VMruntime.network = nil
		}
	})
	return &VMruntime
}

func (r *UnikernelRuntime) CloseLibVirtConnection() {
	r.channelLock.Lock()
	IDs := reflect.ValueOf(r.killQueue).MapKeys()
	r.channelLock.Unlock()
	for _, id := range IDs {
		logger.InfoLogger().Printf("Stopping VM %s : %s %d\n", id.String(), extractSnameFromTaskID(id.String()), extractInstanceNumberFromTaskID(id.String()))
		err := r.Undeploy(extractSnameFromTaskID(id.String()), extractInstanceNumberFromTaskID(id.String()))
		if err != nil {
			logger.ErrorLogger().Printf("Unable to undeploy %s, error: %v", id.String(), err)
		}
	}
	r.network.Free()
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
		r.channelLock.Unlock()
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
	var err error
	var addr int
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

	if r.network != nil {

		for i := 2; i < 255; i++ {
			_, found := r.IPaddresses[i]
			if !found {
				r.IPaddresses[i] = i
				addr = i
				break
			}
		}
		Bridgename, err := r.network.GetBridgeName()
		domainString, err = CreateXMLDomain(hostname, 1, 64, 1, Bridgename, "x86_64", r.subnet, addr)
		if err != nil {
			logger.ErrorLogger().Printf("VM configuration creation failed: %v", err)
		}
	}

	//Create and start Virtual Machine
	domain, err = r.libVirtConnection.DomainCreateXML(domainString, 0)
	if err != nil {
		revert(err)
		return
	}

	defer func(domain *libvirt.Domain) {
		logger.InfoLogger().Printf("Trying to kill VM %s", hostname)
		err := domain.Destroy()
		if r.network != nil {
			delete(r.IPaddresses, addr)
		}
		//r.channelLock.Lock()
		//defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
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

func CreateXMLDomain(Hostname string, DomainID int, Memory uint, CoreCount uint, BrigeName string, Arch string, Subnet string, Addr int) (string, error) {
	var pci_index uint = 0

	CMDString := fmt.Sprintf("netdev.ipv4_addr=%s%d netdev.ipv4_gw_addr=%s1  netdev.ipv4_subnet_mask=255.255.255.0 --", Subnet, Addr, Subnet)
	domain := &libvirtxml.Domain{
		Name:          Hostname,
		Type:          "kvm",
		ID:            &DomainID,
		Title:         Hostname,
		Memory:        &libvirtxml.DomainMemory{Value: Memory, Unit: "MiB"},
		CurrentMemory: &libvirtxml.DomainCurrentMemory{Value: Memory, Unit: "MiB"},
		VCPU:          &libvirtxml.DomainVCPU{Placement: "static", Value: CoreCount},
		CPU:           &libvirtxml.DomainCPU{Mode: "host-passthrough", Check: "none", Migratable: "on"},
		OnCrash:       "restart",
		OnPoweroff:    "destroy",
		OnReboot:      "restart",
		Devices: &libvirtxml.DomainDeviceList{
			//Emulator: "/usr/bin/qemu-system-x86_64",
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
			Type:   &libvirtxml.DomainOSType{Type: "hvm" /*, Arch: Arch*/},
			Kernel: "/home/orch/shared/kernel_images/httpreply_kvm-x86_64",
			BootDevices: []libvirtxml.DomainBootDevice{
				{
					Dev: "hd",
				},
			},
			Cmdline: CMDString,
		},
	}
	x, err := domain.Marshal()
	logger.InfoLogger().Printf("%s", x)

	return x, err
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
