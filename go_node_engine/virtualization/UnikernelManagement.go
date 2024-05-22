package virtualization

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	rt "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-qemu/qmp"
	"github.com/struCoder/pidusage"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

type qemuDomain struct {
	Name        string
	Sname       string
	Instance    int
	qemuProcess *os.Process
}

type UnikernelRuntime struct {
	qemuCommand string
	qemuPath    string
	qemuDomains map[string]*qemuDomain
	killQueue   map[string]*chan bool
	channelLock *sync.RWMutex
}

var ukruntime = UnikernelRuntime{
	channelLock: &sync.RWMutex{},
}

var ukSyncOnce sync.Once

func RegisterUnikernelQemuRuntime() *UnikernelRuntime {
	ukSyncOnce.Do(func() {
		var command string
		var err error

		//Check which version of qemu is required for kvm support (local arch)
		if rt.GOARCH == "amd64" {
			command = "qemu-system-x86_64"
		} else {
			command = "qemu-system-aarch64"
		}

		path, err := exec.LookPath(command)
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to find qemu executable(%s): %v\n", command, err)
			ukruntime.qemuPath = ""
		}
		ukruntime.qemuPath = path
		logger.InfoLogger().Printf("Using qemu at %s\n", path)
		ukruntime.killQueue = make(map[string]*chan bool)
		ukruntime.qemuDomains = make(map[string]*qemuDomain)
		_ = os.MkdirAll("/tmp/node_engine/kernel/tmp/", 0755)
		_ = os.MkdirAll("/tmp/node_engine/inst/", 0755)
		model.GetNodeInfo().AddSupportedTechnology(model.UNIKERNEL_RUNTIME)
		RegisterRuntime(model.UNIKERNEL_RUNTIME, &ukruntime)
	})
	return &ukruntime
}

func (r *UnikernelRuntime) StopUnikernelRuntime() {
	r.channelLock.Lock()
	IDs := reflect.ValueOf(r.killQueue).MapKeys()
	r.channelLock.Unlock()
	for _, id := range IDs {
		if r.killQueue[id.String()] == nil {
			continue
		}
		logger.InfoLogger().Printf("Stopping VM %s : %s %d\n", id.String(), extractSnameFromTaskID(id.String()), extractInstanceNumberFromTaskID(id.String()))
		err := r.Undeploy(extractSnameFromTaskID(id.String()), extractInstanceNumberFromTaskID(id.String()))
		if err != nil {
			logger.ErrorLogger().Printf("Unable to undeploy %s, error: %v", id.String(), err)
		}
	}

	logger.InfoLogger().Print("Stopped all Unikernel deployments\n")
}

func (r *UnikernelRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {

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

var path = "/tmp/node_engine/kernel/"
var inst_path = "/tmp/node_engine/inst/"

/*
Load the Unikernel from the URL given or used cached version.
Return:
*/

func GetKernelImage(kernel string, name string, sname string) *string {

	//filename := strings.ReplaceAll(kernel, "/", "_")
	kernel_tar := path + sname + ".tar.gz"
	kernel_location := path + sname + "/"
	instance_path := inst_path + name
	kernel_local := kernel_location + "kernel"

	/*This is to make sure that in case of a redeployment
	Makes sure that the directory does not already exists
	and waits if it does*/

	_, err := os.Stat(instance_path)
	if !errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	os.Mkdir(instance_path, 0777)
	var kimage *os.File
	_, err = os.Stat(kernel_tar)
	if err != nil {
		logger.InfoLogger().Printf("Kernel not found locally")
		kimage, err = os.Create(kernel_tar)

		if err != nil {
			logger.InfoLogger().Printf("Unable to create Kernel: %v", err)
			return nil
		}
		defer kimage.Close()

		d, err := http.Get(kernel)
		if err != nil {
			logger.InfoLogger().Printf("Unable to locate kernel image (%s): %v", kernel, err)
			os.Remove(kernel_tar)
			return nil
		}
		size, err := io.Copy(kimage, d.Body)
		if err != nil {
			logger.InfoLogger().Printf("Kernel download failed: %v", err)
			os.Remove(kernel_tar)
			return nil
		}
		d.Body.Close()
		logger.InfoLogger().Printf("Written %d B", size)
		kimage.Close()

		os.Mkdir(kernel_location, 0777)
		/*unpack Kernel and additional data*/
		kimage, _ = os.Open(kernel_tar)
		defer kimage.Close()

		exdata, err := gzip.NewReader(kimage)

		tardata := tar.NewReader(exdata)

		for true {
			header, err := tardata.Next()

			if err != nil {
				if err == io.EOF {
					break
				}
				logger.InfoLogger().Printf("Unable to read tar: %v", err)
				return nil
			}

			switch header.Typeflag {
			case tar.TypeDir:
				err := os.Mkdir(kernel_location+header.Name, 0777)
				if err != nil {
					logger.InfoLogger().Printf("Unable to create dir: %v", err)
				}
			case tar.TypeReg:
				file, err := os.Create(kernel_location + header.Name)
				file.Chmod(header.FileInfo().Mode())
				if err != nil {
					logger.InfoLogger().Printf("Unable to create file: %v", err)
				}
				_, err = io.Copy(file, tardata)
				if err != nil {
					logger.InfoLogger().Printf("File copy failed: %v", err)
				}
			default:
				logger.InfoLogger().Printf("ERROR: incorrect typeflag")
				return nil

			}

		}
	} else {
		logger.InfoLogger().Printf("Kernel found locally")
	}
	if err != nil {
		logger.InfoLogger().Printf("Unable to open kernel archive: %v", err)
		return nil
	}

	_, err = os.Stat(kernel_location + "files")
	if !errors.Is(err, fs.ErrNotExist) {
		logger.InfoLogger().Printf("Creating new instance envioument %s -> %s", kernel_location+"files", instance_path)

		err = exec.Command("cp", "-r", kernel_location+"files", instance_path).Run()
		if err != nil {
			logger.InfoLogger().Printf("Unable to set files: %v", err)
		}
	}

	_, err = os.Stat(kernel_location + "files1")
	if !errors.Is(err, fs.ErrNotExist) {
		logger.InfoLogger().Printf("Creating new instance envioument %s -> %s", kernel_location+"files1", instance_path+"/files")

		err = exec.Command("cp", "-r", kernel_location+"files1", instance_path).Run()
		if err != nil {
			logger.InfoLogger().Printf("Unable to set files: %v", err)
		}

		err = exec.Command("mv", instance_path+"/files1", instance_path+"/files").Run()
		if err != nil {
			logger.InfoLogger().Printf("Unable to set files: %v", err)
		}

	}

	_, err = os.Stat(kernel_location + "initramfs.cpio")
	if !errors.Is(err, fs.ErrNotExist) {
		logger.InfoLogger().Printf("Creating new ramdisk envioument %s -> %s", kernel_location+"initramfs.cpio", instance_path+"/initramfs.cpio")

		err = exec.Command("cp", "-r", kernel_location+"initramfs.cpio", instance_path).Run()
		if err != nil {
			logger.InfoLogger().Printf("Unable to set files: %v", err)
		}

	}

	//Kernel image is expected at a fixed location within the archive ./kernel
	_, err = os.Stat(kernel_local)
	if err != nil {
		logger.InfoLogger().Printf("Archive does not seem to contain the kernel image: %v", err)
		return nil
	}
	logger.InfoLogger().Printf("Kernel location: %s", kernel_local)

	return &instance_path
}

func getUnikernelURL(position int, code string) string {
	addr := strings.Split(code, ",")
	logger.InfoLogger().Printf("%v", addr)
	if position >= len(addr) {
		return ""
	}
	return addr[position]
}

func (r *UnikernelRuntime) VirtualMachineCreationRoutine(
	service model.Service,
	killChannel *chan bool,
	startup chan bool,
	errorchan chan error,
	statusChangeNotificationHandler func(service model.Service),
) {
	var qemuConfig QemuConfiguration
	var qemuMonitor *qmp.SocketMonitor

	qemuConfig.Memory = service.Memory
	qemuConfig.CPU = service.Vcpus
	//hostname is used as name for the namespace in which the unikernel will be running in
	hostname := genTaskID(service.Sname, service.Instance)
	qemuConfig.Name = hostname
	qemuConfig.NSname = &hostname

	var kernelImage string = ""

	for i, a := range service.Architectures {
		if a == rt.GOARCH {
			kernelImage = getUnikernelURL(i, service.Image)
		}
	}

	if kernelImage == "" {
		logger.InfoLogger().Printf("Failed to find kernel/architecture pair.")
	}

	kernelPath := GetKernelImage(kernelImage, hostname, service.Sname)
	if kernelPath == nil {
		logger.InfoLogger().Println("Failed to get Kernel image")
		return
	}
	qemuConfig.Kernel = path + service.Sname + "/kernel"

	qemuConfig.Instancepath = *kernelPath

	revert := func(err error, instance string) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
		os.RemoveAll(inst_path + instance)
		logger.InfoLogger().Printf("Removing Instance data -- ")
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
	}

	qemuConfig.KernelArgs = service.Commands

	//Generate the command to start Qemu with
	command, args, err := qemuConfig.GenerateArgs(r)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to start qemu: %v", err)
		revert(err, hostname)
		return
	}

	qemuCmd = exec.Command(command, args...)

	logger.InfoLogger().Printf("Unikernel starting command: %s", qemuCmd.String())

	err = qemuCmd.Start()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to start qemu: %v", err)
		revert(err, hostname)
		return
	}
	logger.InfoLogger().Println("Unikernel started")

	exitStatusQemu := make(chan int)

	go func(status chan int) {
		err = qemuCmd.Wait()
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				logger.InfoLogger().Printf("Qemu exited with code %d and error %s", e.ExitCode(), string(e.Stderr))
				status <- e.ExitCode()
			} else {
				logger.InfoLogger().Printf("Unexpected error occured %v", err)
				status <- -1
			}
		} else {
			status <- 0
		}
	}(exitStatusQemu)

	Domain := qemuDomain{
		Name:        hostname,
		Sname:       service.Sname,
		Instance:    service.Instance,
		qemuProcess: qemuCmd.Process,
	}

	socketPath := fmt.Sprintf("%s/%s", qemuConfig.Instancepath, hostname)
	for i := 0; i < 3; i++ {
		//Wait for qemu to properly start up maximum 3 times
		conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)

		if errors.Is(err, os.ErrNotExist) {
			//logger.InfoLogger().Printf("Waiting: %v", err)
			time.Sleep(10 * time.Millisecond)
		} else if err != nil {
			if !strings.HasSuffix(err.Error(), ": connection refused") {
				logger.InfoLogger().Printf("Something went wrong while starting Qemu %v", err)
				revert(err, hostname)
				if model.GetNodeInfo().Overlay {
					err = requests.DeleteNamespaceForUnikernel(service.Sname, service.Instance)
					if err != nil {
						logger.InfoLogger().Printf("Unable to undeploy %s's network: %v", hostname, err)
					}
				}
				if qemuCmd.Process != nil {
					qemuCmd.Process.Kill()
				}
				return
			}
			//logger.InfoLogger().Printf("Waiting: %v", err)

		} else {
			conn.Close()
			break
		}

	}

	logger.InfoLogger().Printf("Trying to connec to to %s", socketPath)
	qemuMonitor, err = qmp.NewSocketMonitor("unix", socketPath, 2*time.Second)
	if err != nil {
		logger.InfoLogger().Printf("Failed to Create connection to QMP: %v\n", err)
		revert(err, hostname)
		if model.GetNodeInfo().Overlay {
			err = requests.DeleteNamespaceForUnikernel(service.Sname, service.Instance)
			if err != nil {
				logger.InfoLogger().Printf("Unable to undeploy %s's network: %v", hostname, err)
			}
		}
		//Kill the qemu process because of no qmp connectivity
		if qemuCmd.Process != nil {
			qemuCmd.Process.Kill()
		}
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

		//Undeploy the network -> Delete Namespace
		if model.GetNodeInfo().Overlay {
			err = requests.DeleteNamespaceForUnikernel(service.Sname, service.Instance)
			if err != nil {
				logger.InfoLogger().Printf("Unable to undeploy %s's network: %v", hostname, err)
			}
		}
		//Delete instance folder
		os.RemoveAll(inst_path + hostname)
		logger.InfoLogger().Printf("Removing Instance data %s", inst_path+hostname)

		if err != nil {
			*killChannel <- false
		} else {
			*killChannel <- true
		}
	}(qemuMonitor)

	startup <- true
	select {
	case return_value := <-exitStatusQemu:
		logger.InfoLogger().Printf("Received status back from Qemu process %d", return_value)
	case <-*killChannel:
		logger.InfoLogger().Printf("Kill channel message received for unikernel")
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
				//Get CPU and memory stats based on pid
				sysInfo, err := pidusage.GetStat(domain.qemuProcess.Pid)
				if err != nil {
					logger.ErrorLogger().Printf("Unable to fetch task info: %v", err)
					continue
				}
				resourceList = append(resourceList, model.Resources{
					Cpu:      fmt.Sprintf("%f", sysInfo.CPU),
					Memory:   fmt.Sprintf("%f", sysInfo.Memory),
					Disk:     fmt.Sprintf("%d", 0),
					Sname:    domain.Sname,
					Logs:     getLogs(domain.Name),
					Runtime:  string(model.UNIKERNEL_RUNTIME),
					Instance: domain.Instance,
				})

			}
			notifyHandler(resourceList)
		}
	}

}

type QemuConfiguration struct {
	Name         string
	Memory       int
	CPU          int
	Instancepath string
	Kernel       string
	KernelArgs   []string
	NSname       *string
}

func (q *QemuConfiguration) GenerateArgs(r *UnikernelRuntime) (string, []string, error) {
	args := make([]string, 0)
	command := r.qemuPath

	//Check if Qemu needs to run in different Namespace
	if model.GetNodeInfo().Overlay {
		command = "ip"
		args = append(args, "netns", "exec", *q.NSname, r.qemuPath)
	}

	name := fmt.Sprintf("%s,debug-threads=on", q.Name)
	args = append(args, "-name", name)

	//Set qemu log folder
	serialiface := fmt.Sprintf("file:%s/%s", model.GetNodeInfo().LogDirectory, q.Name)
	args = append(args, "-serial", serialiface)

	//Kernel image
	//kernel := q.Instancepath + "kernel"
	args = append(args, "-kernel", q.Kernel, "-nographic", "-nodefaults", "-no-user-config")

	//Memory and CPU
	memory := fmt.Sprintf("%d", q.Memory)
	args = append(args, "-m", memory, "-smp", fmt.Sprintf("%d", q.CPU))

	// Attatch network if overlay mode ativated
	if model.GetNodeInfo().Overlay {

		//Get the default route for the namespace
		ip, gw, mask, mac, fd, err := deleteDefaultIpGwMask(*q.NSname)
		if err != nil {
			logger.InfoLogger().Printf("Unable to get default route for namespace %s: %v", *q.NSname, err)
			return "", []string{}, err
		}

		//Network
		if model.GetNodeInfo().Overlay {
			//Network backend fixed at tap0 and virbr0 created inside the namespace "tap,id=n0,fd=" + strconv.Itoa(devFd),
			args = append(args, "-netdev", "tap,id=tap0,vhost=on,fd="+strconv.Itoa(fd))
			//Network device
			args = append(args, "-device", "virtio-net-pci,id=nic1,addr=0x0a,netdev=tap0,mac="+mac)
		}

		//Kernel arguments including the network configuration
		//The arguments after -- are given to the main function of the unikernel
		args = append(args, "-append")
		KernelArgsStr := " "
		for _, kernelarg := range q.KernelArgs {
			KernelArgsStr += kernelarg + " "
		}
		args = append(args, "netdev.ipv4_addr="+ip+" netdev.ipv4_gw_addr="+gw+" netdev.ipv4_subnet_mask="+mask+" --"+KernelArgsStr)

	}

	//Check if a folder is to be mounted
	mountpath := fmt.Sprintf("%s/files/", q.Instancepath)
	_, err := os.Stat(mountpath)
	if err == nil {
		logger.InfoLogger().Println("Mounting as folder for unikernel")

		//FS backend
		fsdevarg := fmt.Sprintf("local,security_model=passthrough,id=hvirtio0,path=%s/files", q.Instancepath)
		args = append(args, "-fsdev", fsdevarg)

		fsdevarg2 := fmt.Sprintf("local,security_model=passthrough,id=hvirtio1,path=%s/files", q.Instancepath)
		args = append(args, "-fsdev", fsdevarg2)

		//FS device
		args = append(args, "-device", "virtio-9p-pci,fsdev=hvirtio0,mount_tag=fs0")
		args = append(args, "-device", "virtio-9p-pci,fsdev=hvirtio1,mount_tag=fs1")
	}
	//Check if ramdisk provided
	initramfs := fmt.Sprintf("%s/initramfs.cpio", q.Instancepath)
	_, err = os.Stat(initramfs)
	if err == nil {
		logger.InfoLogger().Println("Mounting .cpio root fs")
		//FS backend
		fsdevarg2 := fmt.Sprintf("local,security_model=passthrough,id=hvirtio1,path=%s/initramfs.cpio", q.Instancepath)
		args = append(args, "-fsdev", fsdevarg2)

		//FS device
		args = append(args, "-device", "virtio-9p-pci,fsdev=hvirtio1,mount_tag=fs1")
	}

	//QMP
	Qmp := fmt.Sprintf("unix:%s/%s,server,nowait", q.Instancepath, q.Name)
	args = append(args, "-qmp", Qmp)

	//Set the cpu to host passthrough and enable kvm
	args = append(args, "-cpu", "host", "-enable-kvm")

	if rt.GOARCH != "amd64" {
		args = append(args, "-machine", "virt")
	}

	return command, args, nil
}

// delete and returns the defaultIp Gateway and Netmask of a given namespace
func deleteDefaultIpGwMask(namespace string) (string, string, string, string, int, error) {
	defaultRouteFilter := &netlink.Route{Dst: nil}
	ip, gw, mask, mac := "", "", "", ""
	fd := -1

	err := execInsideNsByName(namespace, func() error {

		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, defaultRouteFilter, netlink.RT_FILTER_DST)
		if err != nil {
			return err
		}
		if n := len(routes); n > 1 {
			return err
		}
		if len(routes) == 0 {
			return err
		}
		route := &routes[0]

		routeIdx := route.LinkIndex
		routelink, err := netlink.LinkByIndex(routeIdx)
		if err != nil {
			return err
		}

		macvtap, err := netlink.LinkByName("tap0")
		if err != nil {
			return err
		}

		addrs, err := netlink.AddrList(routelink, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if len(addrs) == 0 {
			return err
		}

		fd, err = getTapFd(namespace)

		ip = addrs[0].IP.String()
		gw = route.Gw.String()
		mac = macvtap.Attrs().HardwareAddr.String()
		mask = net.IP(addrs[0].Mask).String()

		if err = netlink.AddrDel(routelink, &addrs[0]); err != nil {
			return err
		}

		return nil
	})

	return ip, gw, mask, mac, fd, err
}

// defaultRoute returns the default route for the current namespace.
func getTapFd(namespace string) (int, error) {
	var fd int

	tapdev, err := netlink.LinkByName("tap0")
	if err != nil {
		return -1, err
	}
	fd, err = unix.Open("/dev/tap"+strconv.Itoa(tapdev.Attrs().Index), unix.O_RDWR, 0)
	if err != nil {
		return -1, err
	}

	return fd, nil
}

// Execute function inside a namespace based on Ns name
func execInsideNsByName(Nsname string, function func() error) error {
	var containerNs netns.NsHandle

	rt.LockOSThread()
	defer rt.UnlockOSThread()

	stdNetns, err := netns.Get()
	if err == nil {
		defer stdNetns.Close()
		containerNs, err = netns.GetFromName(Nsname)
		if err == nil {
			defer netns.Set(stdNetns)
			err = netns.Set(containerNs)
			if err == nil {
				err = function()
			}
		}
	}
	return err
}
