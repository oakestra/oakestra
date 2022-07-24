package virtualization

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"reflect"
	"sync"

	"time"

	"os/exec"

	"github.com/digitalocean/go-qemu/qmp"
)

type UnikernelRuntime struct {
	qemuPath    string
	qemuDomains []string
	killQueue   map[string]*chan bool
	channelLock *sync.RWMutex
}

var VMruntime = UnikernelRuntime{
	channelLock: &sync.RWMutex{},
}

const local_qemu_architecutre = "qemu-system-x86_64"

var libVirtSyncOnce sync.Once

func GetUnikernelRuntime() *UnikernelRuntime {
	libVirtSyncOnce.Do(func() {
		path, err := exec.LookPath(local_qemu_architecutre)
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to find qemu executable(%s): %v\n", local_qemu_architecutre, err)
		}
		VMruntime.qemuPath = path
		logger.InfoLogger().Printf("Using qemu at %s\n", path)
		VMruntime.killQueue = make(map[string]*chan bool)

	})
	return &VMruntime
}

func (r *UnikernelRuntime) StopUnikernelRuntime() {
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

	logger.InfoLogger().Print("Stopped all Unikernel deployments\n")
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
	r.qemuDomains = append(r.qemuDomains, hostname)
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

type QemuConfiguration struct {
	hostname string
	memory   int
}

type QemuStopResult struct {
	ID     string `json:"id"`
	Return struct {
	}
}

func (r *UnikernelRuntime) VirtualMachineCreationRoutine(
	service model.Service,
	killChannel *chan bool,
	startup chan bool,
	errorchan chan error,
	statusChangeNotificationHandler func(service model.Service),
) {

	var qemuName string
	var qemuNetwork string
	var qemuQmp string
	var qemuMemory string
	var qemuKernel string
	var qemuAdditinalParameters string = "-cpu host -enable-kvm -daemonize -nographics"
	var qemuMonitor *qmp.SocketMonitor

	var unikraftKernelArguments string

	qemuMemory = fmt.Sprintf("-m %d", service.Memory)
	qemuKernel = fmt.Sprintf("-kernel %s", service.Image)

	//hostname is used as name for the namespace in which the unikernel will be running in
	hostname := genTaskID(service.Sname, service.Instance)
	qemuName = fmt.Sprintf("-name %s,debug-threads=on", hostname)
	qemuQmp = fmt.Sprintf("-qmp -qmp unix:./%s,server,nowait", hostname)
	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
	}

	if model.GetNodeInfo().Overlay {
		//Use Overlay Network to configure network
		err := requests.CreateNetworkNamespaceForUnikernel(service.Sname, service.Instance, service.Ports)
		if err != nil {
			logger.InfoLogger().Printf("Network creation for Unikernel failed: %v\n", err)
			return
		}
		qemuNetwork = fmt.Sprintf("-netdev tap,id=tap0,ifname=tap0,script=no,downscript=no,br=virbr0 -device virtio-net,netdev=tap0,mac=52:55:00:d1:55:01")
		unikraftKernelArguments = "-append \"netdev.ipv4_addr=192.168.1.2 nedev.ipv4_gw_addr=192.168.1.1 netdev.ipv4_subnet_mask=255.255.255.0 --\""
	} else {
		//Start Unikernel without network
		qemuNetwork = ""
		unikraftKernelArguments = ""
	}

	//Create and start Virtual Machine
	qemuCmd := exec.Command(r.qemuPath, qemuName, qemuKernel, unikraftKernelArguments, qemuMemory, qemuNetwork, qemuQmp, qemuAdditinalParameters)
	err := qemuCmd.Run()
	if err != nil {
		revert(err)
		return
	}
	socketPath := fmt.Sprintf("./%s", hostname)
	qemuMonitor, err = qmp.NewSocketMonitor("unix", socketPath, 2*time.Second)
	if err != nil {
		logger.InfoLogger().Printf("Failed to Create connection to QMP: %v\n", err)
		revert(err)
		return
	}

	defer func(monitor *qmp.SocketMonitor) {
		logger.InfoLogger().Printf("Trying to kill VM %s", hostname)

		//There is no guaranteed answer for the quit Command
		cmd := []byte(`{"execute": "quit"}`)
		err := monitor.Connect()
		monitor.Run(cmd)
		monitor.Disconnect()
		//r.channelLock.Lock()
		//defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
		if err != nil {
			*killChannel <- false
		} else {
			*killChannel <- true
		}
	}(qemuMonitor)

	startup <- true

	select {
	case <-*killChannel:
		logger.InfoLogger().Printf("Kill channel message received for unikernel")
		_ = requests.DeleteNamespaceForUnikernel(service.Sname, service.Instance)
	}
	service.Status = model.SERVICE_DEAD
	statusChangeNotificationHandler(service)
}

func (r *UnikernelRuntime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {

	for true {
		select {
		case <-time.After(every):
			resourceList := make([]model.Resources, 0)
			for _, domain := range r.qemuDomains {
				resourceList = append(resourceList, model.Resources{
					Cpu:      fmt.Sprintf("%f", 0.0),
					Memory:   fmt.Sprintf("%f", 100.0),
					Disk:     fmt.Sprintf("%f", 0.0),
					Sname:    extractSnameFromTaskID(domain),
					Runtime:  model.UNIKERNEL_RUNTIME,
					Instance: extractInstanceNumberFromTaskID(domain),
				})

			}
			notifyHandler(resourceList)
		}
	}

}
