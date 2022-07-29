package virtualization

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"os"
	"reflect"
	"sync"

	"time"

	"os/exec"

	"github.com/digitalocean/go-qemu/qmp"
	"github.com/struCoder/pidusage"
)

type qemuDomain struct {
	Name        string
	Sname       string
	Instance    int
	qemuProcess *os.Process
}

type UnikernelRuntime struct {
	qemuPath    string
	qemuDomains map[string]*qemuDomain
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
		VMruntime.qemuDomains = make(map[string]*qemuDomain)

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
	logger.InfoLogger().Println("Start Unikernel creation")
	go r.VirtualMachineCreationRoutine(service, &killChannel, startupChannel, errorChannel, statusChangeNotificationHandler)

	if <-startupChannel != true {
		logger.InfoLogger().Printf("faield nognsfonojsfnbofnbndbodbndfobnjodgndobng\n\n")
		return <-errorChannel
	}

	return nil
}

func (r *UnikernelRuntime) Undeploy(service string, instance int) error {

	r.channelLock.Lock()
	defer r.channelLock.Unlock()
	hostname := genTaskID(service, instance)
	//r.qemuDomains = append(r.qemuDomains, hostname)
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
	//var qemuNetwork string
	//var qemuQmp string
	var qemuMemory string
	//var qemuKernel string
	//var qemuAdditinalParameters string = "-cpu host -enable-kvm -nographic" //-daemonize
	var qemuMonitor *qmp.SocketMonitor

	var unikraftKernelArguments string

	qemuMemory = fmt.Sprintf("%d", service.Memory)
	//qemuKernel = fmt.Sprintf("-kernel", service.Image)

	//hostname is used as name for the namespace in which the unikernel will be running in
	hostname := genTaskID(service.Sname, service.Instance)
	qemuName = fmt.Sprintf("%s,debug-threads=on", hostname)
	qemuQmp := fmt.Sprintf("unix:./%s,server,nowait", hostname)
	revert := func(err error) {
		logger.InfoLogger().Printf("FAILED TO START")
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
	}
	var qemuCmd *exec.Cmd
	var err error
	if model.GetNodeInfo().Overlay {
		//Use Overlay Network to configure network
		err := requests.CreateNetworkNamespaceForUnikernel(service.Sname, service.Instance, service.Ports)
		if err != nil {
			logger.InfoLogger().Printf("Network creation for Unikernel failed: %v\n", err)
			return
		}
		//qemuNetwork = fmt.Sprintf("-netdev tap,id=tap0,ifname=tap0,script=no,downscript=no,br=virbr0 -device virtio-net,netdev=tap0,mac=52:55:00:d1:55:01")
		unikraftKernelArguments = "\"netdev.ipv4_addr=192.168.1.2 nedev.ipv4_gw_addr=192.168.1.1 netdev.ipv4_subnet_mask=255.255.255.0 --\""
		//Command uses ip-netns to run in a different namespace
		qemuCmd = exec.Command("ip", "netns", "exec", hostname, r.qemuPath, "-name", qemuName, "-kernel", service.Image, "-append", unikraftKernelArguments, "-m", qemuMemory, "-netdev",
			"tap,id=tap0,ifname=tap0,script=no,downscript=no,br=virbr0", "-device", "virtio-net,netdev=tap0,mac=52:55:00:d1:55:01",
			"-cpu", "host", "-enable-kvm" /*"-nographic",*/, "-qmp", qemuQmp)

	} else {
		//Start Unikernel without network
		//qemuNetwork = ""
		unikraftKernelArguments = ""
		qemuCmd = exec.Command(r.qemuPath, "-name", qemuName, "-kernel", service.Image, "-append", unikraftKernelArguments, "-m",
			qemuMemory, "-cpu", "host", "-enable-kvm" /*, "-nographic"*/, "-qmp", qemuQmp)

	}

	logger.InfoLogger().Printf(qemuCmd.String())
	logger.InfoLogger().Printf("STARTING UNIKERNEL")
	err = qemuCmd.Start()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to start qemu: %v", err)
		revert(err)
		return
	}
	logger.InfoLogger().Printf("NOW RUNNING UNIKERNEL")
	//Create and start Virtual Machine
	//qemuCmd = exec.Command(r.qemuPath, qemuName, qemuKernel, unikraftKernelArguments, qemuMemory, qemuNetwork, qemuQmp, qemuAdditinalParameters)
	//qemuCmd = exec.Command(r.qemuPath, "-name", qemuName, "-kernel", service.Image, "-append", unikraftKernelArguments, "-m", qemuMemory)
	//qemuCmd = exec.Command(r.qemuPath, "-kernel", service.Image)

	//out, err := qemuCmd.CombinedOutput()
	//logger.InfoLogger().Printf("%s", out)

	Domain := qemuDomain{
		Name:        hostname,
		Sname:       service.Sname,
		Instance:    service.Instance,
		qemuProcess: qemuCmd.Process,
	}

	time.Sleep(100 * time.Millisecond) // Wait for qemu to properly init
	socketPath := fmt.Sprintf("./%s", hostname)
	logger.InfoLogger().Printf("Trying to connecto to %s", socketPath)
	qemuMonitor, err = qmp.NewSocketMonitor("unix", socketPath, 2*time.Second)
	if err != nil {
		logger.InfoLogger().Printf("Failed to Create connection to QMP: %v\n", err)
		revert(err)
		return
	}

	//Add Domain
	r.channelLock.Lock()
	r.qemuDomains[hostname] = &Domain
	r.channelLock.Unlock()

	defer func(monitor *qmp.SocketMonitor) {
		logger.InfoLogger().Printf("Trying to kill VM %s", hostname)
		//There is no guaranteed answer for the quit Command
		cmd := []byte(`{"execute": "quit"}`)
		err := monitor.Connect()

		if err != nil {
			logger.InfoLogger().Printf("Failed to connect to qmp: %v", err)
		}
		_, err = monitor.Run(cmd)
		if err != nil {
			logger.InfoLogger().Printf("Failed to close qemu: %v\n", err)
		}
		err = monitor.Disconnect()
		if err != nil {
			logger.InfoLogger().Printf("Failed to close connection (expected): %v", err)
		}

		r.channelLock.Lock()
		r.killQueue[hostname] = nil
		delete(r.qemuDomains, hostname)
		r.channelLock.Unlock()
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
			for k, _ := range r.qemuDomains {
				logger.InfoLogger().Printf("\n\n")

				logger.InfoLogger().Printf("%s", k)

				logger.InfoLogger().Printf("\n\n")

			}
			for _, domain := range r.qemuDomains {
				//Get CPU and memory stats based on pid
				sysInfo, err := pidusage.GetStat(domain.qemuProcess.Pid)
				if err != nil {
					logger.ErrorLogger().Printf("Unable to fetch task info: %v", err)
					continue
				}
				logger.InfoLogger().Printf("%f", sysInfo.CPU)
				resourceList = append(resourceList, model.Resources{
					Cpu:      fmt.Sprintf("%f", 0.0 /*sysInfo.CPU*/),
					Memory:   fmt.Sprintf("%f", 0.0 /*sysInfo.Memory <- Includes memory overhead of qemu itself -> reports more than allowed*/),
					Disk:     fmt.Sprintf("%d", 0),
					Sname:    domain.Sname,
					Runtime:  model.UNIKERNEL_RUNTIME,
					Instance: domain.Instance,
				})

			}
			notifyHandler(resourceList)
		}
	}

}

type QemuConfiguration struct {
	Name       string
	Memory     int
	Kernelpath string
}
