package virtualization

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/contrib/nvidia"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/shirou/gopsutil/docker"
	"github.com/shirou/gopsutil/process"
	"github.com/struCoder/pidusage"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ContainerRuntime struct {
	contaierClient *containerd.Client
	killQueue      map[string]*chan bool
	channelLock    *sync.RWMutex
	ctx            context.Context
}

var runtime = ContainerRuntime{
	channelLock: &sync.RWMutex{},
}

var containerdSingletonCLient sync.Once
var startContainerMonitoring sync.Once

const NAMESPACE = "oakestra"
const CGROUPV1_BASE_MEM = "/sys/fs/cgroup/memory/" + NAMESPACE
const CGROUPV2_BASE_MEM = "/sys/fs/cgroup/" + NAMESPACE

func GetContainerdClient() *ContainerRuntime {
	containerdSingletonCLient.Do(func() {
		client, err := containerd.New("/run/containerd/containerd.sock")
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to start the container engine: %v\n", err)
		}
		runtime.contaierClient = client
		runtime.killQueue = make(map[string]*chan bool)
		runtime.ctx = namespaces.WithNamespace(context.Background(), NAMESPACE)
		runtime.forceContainerCleanup()
		model.GetNodeInfo().AddSupportedTechnology(model.CONTAINER_RUNTIME)
	})
	return &runtime
}

func (r *ContainerRuntime) StopContainerdClient() {
	r.channelLock.Lock()
	taskIDs := reflect.ValueOf(r.killQueue).MapKeys()
	r.channelLock.Unlock()

	for _, taskid := range taskIDs {
		err := r.Undeploy(extractSnameFromTaskID(taskid.String()), extractInstanceNumberFromTaskID(taskid.String()))
		if err != nil {
			logger.ErrorLogger().Printf("Unable to undeploy %s, error: %v", taskid.String(), err)
		}
	}
	r.contaierClient.Close()
}

func (r *ContainerRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {

	var image containerd.Image
	// pull the given image
	sysimg, err := r.contaierClient.ImageService().Get(r.ctx, service.Image)
	if err == nil {
		image = containerd.NewImage(r.contaierClient, sysimg)
	} else {
		logger.ErrorLogger().Printf("Error retrieving the image: %v \n Trying to pull the image online.", err)

		image, err = r.contaierClient.Pull(r.ctx, service.Image, containerd.WithPullUnpack)
		if err != nil {
			return err
		}
	}

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

	// create startup routine which will accompany the container through its lifetime
	go r.containerCreationRoutine(
		r.ctx,
		image,
		service,
		startupChannel,
		errorChannel,
		&killChannel,
		statusChangeNotificationHandler,
	)

	// wait for updates regarding the container creation
	if <-startupChannel != true {
		return <-errorChannel
	}

	return nil
}

func (r *ContainerRuntime) Undeploy(service string, instance int) error {
	r.channelLock.Lock()
	defer r.channelLock.Unlock()
	taskid := genTaskID(service, instance)
	el, found := r.killQueue[taskid]
	if found && el != nil {
		logger.InfoLogger().Printf("Sending kill signal to %s", taskid)
		*r.killQueue[taskid] <- true
		select {
		case res := <-*r.killQueue[taskid]:
			if res == false {
				logger.ErrorLogger().Printf("Unable to stop service %s", taskid)
			}
		case <-time.After(5 * time.Second):
			logger.ErrorLogger().Printf("Unable to stop service %s", taskid)
		}
		delete(r.killQueue, taskid)
		return nil
	}
	return errors.New("service not found")
}

func (r *ContainerRuntime) containerCreationRoutine(
	ctx context.Context,
	image containerd.Image,
	service model.Service,
	startup chan bool,
	errorchan chan error,
	killChannel *chan bool,
	statusChangeNotificationHandler func(service model.Service),
) {

	hostname := genTaskID(service.Sname, service.Instance)

	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
	}

	//create container general oci specs
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostHostsFile,
		oci.WithHostname(hostname),
		oci.WithEnv(append([]string{fmt.Sprintf("HOSTNAME=%s", hostname)}, service.Env...)),
	}
	//add user defined commands
	if len(service.Commands) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(service.Commands...))
	}
	//add GPU if needed
	if service.Vgpus > 0 {
		specOpts = append(specOpts, nvidia.WithGPUs(nvidia.WithDevices(0), nvidia.WithAllCapabilities))
		logger.InfoLogger().Printf("NVIDIA - Adding GPU driver")
	}
	//add resolve file with default google dns
	resolvconfFile, err := getGoogleDNSResolveConf()
	if err != nil {
		revert(err)
		return
	}
	defer resolvconfFile.Close()
	_ = resolvconfFile.Chmod(444)
	specOpts = append(specOpts, withCustomResolvConf(resolvconfFile.Name()))

	// create the container
	container, err := r.contaierClient.NewContainer(
		ctx,
		hostname,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(fmt.Sprintf("%s-snapshotter", hostname), image),
		containerd.WithNewSpec(specOpts...),
	)
	if err != nil {
		revert(err)
		return
	}

	//	start task with /tmp/hostname default log directory
	file, err := os.OpenFile(fmt.Sprintf("%s/%s", model.GetNodeInfo().LogDirectory, hostname), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		revert(err)
		return
	}
	defer file.Close()
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStreams(nil, file, file)))

	if err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task creation failure: %v", err)
		_ = container.Delete(ctx)
		revert(err)
		return
	}
	defer func(ctx context.Context, task containerd.Task) {
		err := killTask(ctx, task, container)
		//removing from killqueue
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
		if err != nil {
			*killChannel <- false
		} else {
			*killChannel <- true
		}
	}(ctx, task)

	// get wait channel
	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task wait failure: %v", err)
		revert(err)
		return
	}

	// if Overlay mode is active then attach network to the task
	if model.GetNodeInfo().Overlay {
		taskpid := int(task.Pid())
		err = requests.AttachNetworkToTask(taskpid, service.Sname, service.Instance, service.Ports)
		if err != nil {
			logger.ErrorLogger().Printf("Unable to attach network interface to the task: %v", err)
			revert(err)
			return
		}
	}

	// execute the image's task
	if err := task.Start(ctx); err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task start failure: %v", err)
		revert(err)
		return
	}

	// adv startup finished
	startup <- true

	// wait for manual task kill or task finish
	select {
	case exitStatus := <-exitStatusC:
		//TODO: container exited, do something, notify to cluster manager
		if err != nil {
			return
		}
		logger.InfoLogger().Printf("WARNING: Container exited with status %d", exitStatus.ExitCode())
		service.StatusDetail = fmt.Sprintf("Container exited with status: %d", exitStatus.ExitCode())
	case <-*killChannel:
		logger.InfoLogger().Printf("Kill channel message received for task %s", task.ID())
	}
	service.Status = model.SERVICE_DEAD
	//detaching network
	if model.GetNodeInfo().Overlay {
		_ = requests.DetachNetworkFromTask(service.Sname, service.Instance)
	}
	statusChangeNotificationHandler(service)
	r.removeContainer(container)
}

func getTotalCpuUsageByPid(pid int32) (float64, error) {
	totCpu := 0.0
	procs, err := process.NewProcess(pid)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: %v", err)
		return 0, err
	}

	children, err := procs.Children()
	if err != nil {
		return 0, err
	}

	for _, child := range children {
		cpuUsage, err := child.CPUPercent()
		if err != nil {
			logger.ErrorLogger().Printf("ERROR: %v", err)
			return 0, err
		}
		totCpu += cpuUsage
	}
	return totCpu / float64(model.GetNodeInfo().CpuCores), nil
}

func (r *ContainerRuntime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {
	//start container monitoring service
	startContainerMonitoring.Do(func() {
		for true {
			select {
			case <-time.After(every):
				deployedContainers, err := r.contaierClient.Containers(r.ctx)
				if err != nil {
					logger.ErrorLogger().Printf("Unable to fetch running containers: %v", err)
				}

				resourceList := make([]model.Resources, 0)

				for _, container := range deployedContainers {
					task, err := container.Task(r.ctx, nil)
					if err != nil {
						logger.ErrorLogger().Printf("Unable to fetch container task: %v", err)
						continue
					}

					cpuUsage, err := getTotalCpuUsageByPid(int32(task.Pid()))
					if err != nil {
						sysInfo, err := pidusage.GetStat(int(task.Pid()))
						if err != nil {
							logger.ErrorLogger().Printf("Unable to fetch task info: %v", err)
							continue
						}
						cpuUsage = sysInfo.CPU / float64(model.GetNodeInfo().CpuCores)
					}

					mem, err := r.getContainerMemoryUsage(container.ID(), int(task.Pid()))
					if err != nil {
						logger.ErrorLogger().Printf("Unable to fetch container Memory: %v", err)
						mem = 0
					}

					containerMetadata, err := container.Info(r.ctx)
					if err != nil {
						logger.ErrorLogger().Printf("Unable to fetch container metadata: %v", err)
						continue
					}
					currentsnapshotter := r.contaierClient.SnapshotService(containerd.DefaultSnapshotter)
					usage, err := currentsnapshotter.Usage(r.ctx, containerMetadata.SnapshotKey)
					if err != nil {
						logger.ErrorLogger().Printf("Unable to fetch task disk usage: %v", err)
						continue
					}

					resourceList = append(resourceList, model.Resources{
						Cpu:      fmt.Sprintf("%f", cpuUsage),
						Memory:   fmt.Sprintf("%f", mem),
						Disk:     fmt.Sprintf("%d", usage.Size),
						Sname:    extractSnameFromTaskID(container.ID()),
						Runtime:  string(model.CONTAINER_RUNTIME),
						Logs:     getLogs(container.ID()),
						Instance: extractInstanceNumberFromTaskID(container.ID()),
					})
				}
				//NOTIFY WITH THE CURRENT CONTAINERS STATUS
				notifyHandler(resourceList)
			}
		}
	})
}

func (r *ContainerRuntime) forceContainerCleanup() {
	deployedContainers, err := r.contaierClient.Containers(r.ctx)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to fetch running containers: %v", err)
	}
	for _, container := range deployedContainers {
		r.removeContainer(container)
	}
}

func (r *ContainerRuntime) removeContainer(container containerd.Container) {
	logger.InfoLogger().Printf("Clenaning up container: %s", container.ID())
	task, err := container.Task(r.ctx, nil)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to fetch container task: %v", err)
	}
	if err == nil {
		err = killTask(r.ctx, task, container)
		if err != nil {
			logger.ErrorLogger().Printf("Unable to fetch kill task: %v", err)
		}
	}
	err = container.Delete(r.ctx)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to delete container: %v", err)
	}
}

func (r *ContainerRuntime) getContainerMemoryUsage(containerID string, pid int) (float64, error) {
	//trying fetching memory using CGROUP_V1 path
	mem, err := docker.CgroupMem(containerID, CGROUPV1_BASE_MEM)
	if err != nil {
		//trying fetching memory using CGROUP_V2 path
		mem, err = docker.CgroupMem(containerID, CGROUPV2_BASE_MEM)
		if err != nil {
			//unable to get memory usage from CGROUPS, likely disabled. Defaulting to PID memory consumption
			sysInfo, err := pidusage.GetStat(pid)
			if err != nil {
				return 0, err
			}
			return sysInfo.Memory, nil
		}
	}
	return float64(mem.MemUsageInBytes), nil
}

func withCustomResolvConf(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      src,
			Options:     []string{"rbind", "ro"},
		})
		return nil
	}
}

func getGoogleDNSResolveConf() (*os.File, error) {
	file, err := ioutil.TempFile("/tmp", "edgeio-resolv-conf")
	if err != nil {
		logger.ErrorLogger().Printf("Unable to create temp resolv file: %v", err)
		return nil, err
	}
	_, err = file.WriteString(fmt.Sprintf("nameserver 8.8.8.8\n"))
	if err != nil {
		logger.ErrorLogger().Printf("Unable to write temp resolv file: %v", err)
		return nil, err
	}
	return file, err
}

func killTask(ctx context.Context, task containerd.Task, container containerd.Container) error {
	//removing the task
	p, err := task.LoadProcess(ctx, task.ID(), nil)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR deleting the task, LoadProcess: %v", err)
		return err
	}
	_, err = p.Delete(ctx, containerd.WithProcessKill)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR deleting the task, Delete: %v", err)
		return err
	}
	_, _ = task.Delete(ctx)
	_ = container.Delete(ctx)

	logger.ErrorLogger().Printf("Task %s terminated", task.ID())
	return nil
}

func extractSnameFromTaskID(taskid string) string {
	sname := taskid
	index := strings.LastIndex(taskid, ".instance")
	if index > 0 {
		sname = taskid[0:index]
	}
	return sname
}

func extractInstanceNumberFromTaskID(taskid string) int {
	instance := 0
	separator := ".instance"
	index := strings.LastIndex(taskid, separator)
	if index > 0 {
		number, err := strconv.Atoi(taskid[index+len(separator)+1:])
		if err == nil {
			instance = number
		}
	}
	return instance
}

func genTaskID(sname string, instancenumber int) string {
	return fmt.Sprintf("%s.instance.%d", sname, instancenumber)
}
