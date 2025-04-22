package virtualization

import (
	"context"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd"
	runcoptions "github.com/containerd/containerd/api/types/runc/options"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/contrib/nvidia"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/plugin"
	docker_remote "github.com/containerd/containerd/remotes/docker"
	containerdcfg "github.com/containerd/containerd/v2/cmd/containerd/server/config"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/shirou/gopsutil/docker"
	"github.com/shirou/gopsutil/process"
	"github.com/struCoder/pidusage"
)

// ContainerRuntime is the struct that describes the container runtime
type ContainerRuntime struct {
	containerClient *containerd.Client
	killQueue       map[string]*chan bool
	channelLock     *sync.RWMutex
	ctx             context.Context
}

var runtime = ContainerRuntime{
	// Lock specific for the container runtime interactions.
	// Only one interaction at a time as we have no guarantee the runtime will handle more.
	channelLock: &sync.RWMutex{},
}

var containerdSingletonCLient sync.Once
var startContainerMonitoring sync.Once

var containerdConfig containerdcfg.Config

// NAMESPACE is the namespace of the runtime
const NAMESPACE = "oakestra"

// CGROUPV1_BASE_MEM is the base memory path for cgroup v1
const CGROUPV1_BASE_MEM = "/sys/fs/cgroup/memory/" + NAMESPACE

// CGROUPV2_BASE_MEM is the base memory path for cgroup v2
const CGROUPV2_BASE_MEM = "/sys/fs/cgroup/" + NAMESPACE

// Containerd config path
const CONTAINERD_CONFIG_PATH = "/etc/containerd/config.toml"

// GetContainerdRuntime returns the container runtime client
func GetContainerdRuntime() Runtime {
	containerdSingletonCLient.Do(func() {
		client, err := containerd.New("/run/containerd/containerd.sock")
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to start the container engine: %v\n", err)
		}
		runtime.containerClient = client
		runtime.killQueue = make(map[string]*chan bool)
		runtime.ctx = namespaces.WithNamespace(context.Background(), NAMESPACE)
		runtime.forceContainerCleanup()
		// register default runtime name
		model.GetNodeInfo().AddSupportedTechnology(model.CONTAINER_RUNTIME)

		//fetch containerd runtime configuration
		checkAdditionalRuntimePlugins()

	})
	return &runtime
}

// checks the containerd config file for additional runtimes and registers them
func checkAdditionalRuntimePlugins() {
	err := containerdcfg.LoadConfig(context.Background(), CONTAINERD_CONFIG_PATH, &containerdConfig)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to load containerd config file: %v", err)
		return
	}
	for _, ctd := range containerdConfig.Plugins {
		ctd, ok := ctd.(map[string]interface{})["containerd"].(map[string]interface{})
		if ok {
			runtimes, ok := ctd["runtimes"].(map[string]interface{})
			if ok {
				for runtimeName, _ := range runtimes {
					logger.InfoLogger().Printf("Adding compatibility custom runtime %s configured in containerd config file %s", runtimeName, CONTAINERD_CONFIG_PATH)
					model.GetNodeInfo().AddSupportedTechnology(model.RuntimeType(runtimeName))
					registerRuntimeLink(runtimeName, GetContainerdRuntime)
				}
			}
		}
	}
}

// StopContainerdClient stops the container runtime client
func (r *ContainerRuntime) Stop() {
	r.channelLock.Lock()
	taskIDs := reflect.ValueOf(r.killQueue).MapKeys()
	r.channelLock.Unlock()

	for _, taskid := range taskIDs {
		err := r.Undeploy(extractSnameFromTaskID(taskid.String()), extractInstanceNumberFromTaskID(taskid.String()))
		if err != nil {
			logger.ErrorLogger().Printf("Unable to undeploy %s, error: %v", taskid.String(), err)
		}
	}
	if err := r.containerClient.Close(); err != nil {
		logger.ErrorLogger().Printf("Unable to close containerd client: %v", err)
	}

}

// Deploy deploys a service
func (r *ContainerRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {
	var image containerd.Image

	// pull the given image
	sysimg, err := r.containerClient.ImageService().Get(r.ctx, service.Image)
	if err == nil {
		image = containerd.NewImage(r.containerClient, sysimg)
	} else {
		logger.ErrorLogger().Printf("Error retrieving the image: %v \n Trying to pull the image online.", err)

		remoteOpt := []containerd.RemoteOpt{containerd.WithPullUnpack}
		if service.Platform != "" {
			remoteOpt = append(remoteOpt, containerd.WithPlatform(service.Platform))
		}
		image, err = r.containerClient.Pull(r.ctx, service.Image, remoteOpt...)

		if err != nil {
			// avoid crashing for HTTP based registires in local infrastructures
			if strings.Contains(err.Error(), "http: server gave HTTP response to HTTPS client") {
				alwaysPlainHTTP := func(string) (bool, error) {
					return true, nil
				}
				ropts := []docker_remote.RegistryOpt{
					docker_remote.WithPlainHTTP(alwaysPlainHTTP),
				}
				resolver := docker_remote.NewResolver(docker_remote.ResolverOptions{
					Hosts: docker_remote.ConfigureDefaultRegistries(ropts...),
				})
				image, err = r.containerClient.Pull(r.ctx, service.Image, containerd.WithPullUnpack, containerd.WithResolver(resolver))
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	startupChannel := make(chan bool)
	errorChannel := make(chan error)

	_, err = r.getContainerByTaskID(genTaskID(service.Sname, service.Instance))
	if err == nil {
		return fmt.Errorf("task already deployed")
	}

	// create startup routine which will accompany the container through its lifetime
	go r.containerCreationRoutine(
		r.ctx,
		image,
		service,
		startupChannel,
		errorChannel,
		statusChangeNotificationHandler,
	)

	// wait for updates regarding the container creation
	if !<-startupChannel {
		return <-errorChannel
	}

	return nil
}

// Undeploy undeploys a service
func (r *ContainerRuntime) Undeploy(service string, instance int) error {
	c, err := r.getContainerByTaskID(genTaskID(service, instance))
	if err == nil {
		_ = r.removeContainer(c)
	}
	return err
}

func (r *ContainerRuntime) containerCreationRoutine(
	ctx context.Context,
	image containerd.Image,
	service model.Service,
	startup chan bool,
	errorchan chan error,
	statusChangeNotificationHandler func(service model.Service),
) {

	taskid := genTaskID(service.Sname, service.Instance)
	hostname := fmt.Sprintf("instance-%d", service.Instance)

	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[taskid] = nil
	}

	// Container options
	containerOpts := []containerd.NewContainerOpts{}
	// -- if custom runtime selected, add it to the container
	if service.Runtime != string(model.CONTAINER_RUNTIME) {
		if strings.Contains("io.containerd", service.Runtime) {
			containerOpts = append(containerOpts, containerd.WithRuntime(service.Runtime, &runcoptions.Options{}))
		} else {
			path, err := exec.LookPath(service.Runtime)
			logger.InfoLogger().Printf("Using custom runtime %s", path)
			if err != nil {
				logger.ErrorLogger().Printf("ERROR: unable to find runtime %s, %v", service.Runtime, err)
			}
			containerOpts = append(containerOpts, containerd.WithRuntime(plugin.RuntimeRuncV2, &runcoptions.Options{BinaryName: path}))
		}
	}
	// -- add custom snapshotter
	containerOpts = append(containerOpts, containerd.WithNewSnapshot(fmt.Sprintf("%s-snapshotter", taskid), image))
	// -- add image
	containerOpts = append(containerOpts, containerd.WithImage(image))

	// ---- Custom container general oci specs
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostHostsFile,
		oci.WithHostname(hostname),
		oci.WithEnv(append([]string{fmt.Sprintf("HOSTNAME=%s", hostname)}, service.Env...)),
	}
	if service.Privileged {
		specOpts = append(specOpts, oci.WithDevices("/dev/fuse", "/dev/fuse", "rwm"))
	}
	// ---- add user defined commands
	if len(service.Commands) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(service.Commands...))
	}
	// ---- add GPU if needed
	if service.Vgpus > 0 {
		specOpts = append(specOpts, nvidia.WithGPUs(nvidia.WithDevices(0), nvidia.WithAllCapabilities))
		logger.InfoLogger().Printf("NVIDIA - Adding GPU driver")
	}
	// ---- add resolve file with default google dns
	resolvconfFile, err := getGoogleDNSResolveConf()
	if err != nil {
		revert(err)
		return
	}
	specOpts = append(specOpts, withCustomResolvConf(resolvconfFile))

	// -- add oci SpecOpts to containerOpts
	containerOpts = append(containerOpts, containerd.WithNewSpec(specOpts...))

	// Create the container
	container, err := r.containerClient.NewContainer(
		ctx,
		taskid,
		containerOpts...,
	)
	if err != nil {
		revert(err)
		return
	}

	//	start task with /tmp/taskid default log directory
	file, err := os.OpenFile(fmt.Sprintf("%s/%s", model.GetNodeInfo().LogDirectory, taskid), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		revert(err)
		return
	}
	//defer file.Close()
	defer func() {
		if err := file.Close(); err != nil {
			logger.ErrorLogger().Printf("Unable to close log file: %v", err)
		}
	}()

	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStreams(nil, file, file)))

	if err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task creation failure: %v", err)
		_ = container.Delete(ctx)
		revert(err)
		return
	}
	defer func(ctx context.Context, task containerd.Task) {
		_ = killTask(ctx, task, container)
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
	exitStatus := <-exitStatusC

	if exitStatus.ExitCode() == 0 && service.OneShot {
		service.Status = model.SERVICE_COMPLETED
	} else {
		service.Status = model.SERVICE_DEAD
	}

	//detaching network
	if model.GetNodeInfo().Overlay {
		_ = requests.DetachNetworkFromTask(service.Sname, service.Instance)
	}

	_ = r.removeContainer(container)
	statusChangeNotificationHandler(service)
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
		for {
			select {
			case <-time.After(every):
				deployedContainers, err := r.containerClient.Containers(r.ctx)
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

					currentsnapshotter := r.containerClient.SnapshotService(containerMetadata.Snapshotter)
					usage, err := currentsnapshotter.Usage(r.ctx, containerMetadata.SnapshotKey)
					if err != nil {
						logger.ErrorLogger().Printf("Unable to fetch task disk usage: %v", err)
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
	deployedContainers, err := r.containerClient.Containers(r.ctx)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to fetch running containers: %v", err)
	}
	for _, container := range deployedContainers {
		_ = r.removeContainer(container)
	}
}

func (r *ContainerRuntime) removeContainer(container containerd.Container) error {
	logger.InfoLogger().Printf("Clenaning up container: %s", container.ID())
	task, err := container.Task(r.ctx, nil)
	if err != nil {
		return fmt.Errorf("Unable to fetch container task: %v", err)
	}
	if err == nil {
		err = killTask(r.ctx, task, container)
		if err != nil {
			return fmt.Errorf("Unable to fetch kill task: %v", err)
		}
	}
	err = container.Delete(r.ctx)
	if err != nil {
		return fmt.Errorf("Unable to delete container: %v", err)
	}
	return nil
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

func getGoogleDNSResolveConf() (string, error) {
	file, err := os.CreateTemp("/tmp", "oakestra-resolv-conf")
	if err != nil {
		logger.ErrorLogger().Printf("Unable to create temp resolv file: %v", err)
		return "", err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.ErrorLogger().Printf("Unable to close temp resolv file: %v", err)
		}
	}()
	_, err = file.WriteString("nameserver 8.8.8.8\n")
	if err != nil {
		logger.ErrorLogger().Printf("Unable to write temp resolv file: %v", err)
		return "", err
	}
	_ = file.Chmod(0444)
	return file.Name(), err
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

func (r *ContainerRuntime) getContainerByTaskID(taskid string) (containerd.Container, error) {
	containers, err := r.containerClient.Containers(r.ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range containers {
		if c.ID() == taskid {
			return c, err
		}
	}
	return nil, fmt.Errorf("container not found")
}
