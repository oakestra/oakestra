package virtualization

import (
	"context"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"go_node_engine/utils"
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

type ContainerService struct {
	model.Service
	container containerd.Container
}

// ContainerRuntime is the struct that describes the container runtime
type ContainerRuntime struct {
	containerClient *containerd.Client
	services        map[string]*ContainerService // for migration purposes
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

// RuntimeRoutineOption is a function that can be used to configure the container runtime client
type RoutineOption func(c *RoutineOptionConfig)
type RoutineOptionConfig struct {
	withContainer      containerd.Container
	withImage          containerd.Image
	withImageStatePath string
}

// GetContainerdRuntime returns the container runtime client
func GetContainerdRuntime() Runtime {
	containerdSingletonCLient.Do(func() {
		client, err := containerd.New("/run/containerd/containerd.sock")
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to start the container engine: %v\n", err)
		}
		runtime.containerClient = client
		runtime.services = make(map[string]*ContainerService)
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
	taskIDs := reflect.ValueOf(r.services).MapKeys()
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

	// pull the given image
	image, err := r.getContainerImage(service)
	if err != nil {
		return err
	}

	// create feedback channels for the container creation routine
	startupChannel := make(chan bool)
	errorChannel := make(chan error)

	// check if the service is already deployed
	_, err = r.getContainerByTaskID(genTaskID(service.Sname, service.Instance))
	if err == nil {
		return fmt.Errorf("task already deployed")
	}
	r.channelLock.Lock()
	val, exists := r.services[genTaskID(service.Sname, service.Instance)]
	r.channelLock.Unlock()
	if exists && val != nil {
		return fmt.Errorf("service %s instance %d is already deployed", service.Sname, service.Instance)
	}

	// create startup routine which will accompany the container through its lifetime
	go r.containerCreationRoutine(
		r.ctx,
		service,
		startupChannel,
		errorChannel,
		statusChangeNotificationHandler,
		WithImage(image),
	)

	// wait for updates regarding the container creation
	if !<-startupChannel {
		return <-errorChannel
	}

	return nil
}

// Undeploy undeploys a service
func (r *ContainerRuntime) Undeploy(service string, instance int) error {

	task, exists := r.services[genTaskID(service, instance)]

	//cleanup service list
	if exists {
		r.channelLock.Lock()
		delete(r.services, genTaskID(service, instance))
		r.channelLock.Unlock()
	}

	//remove container by task ID
	c, err := r.getContainerByTaskID(genTaskID(service, instance))
	if err == nil && c != nil {
		_ = r.removeContainer(c)
	} else {
		if task != nil {
			if task.container != nil {
				r.removeContainer(task.container)
			}
		}
	}

	return err
}

// containerCreationRoutine is the routine that generates, executes and monitors the main task of a container
// It is called by the Deploy method and runs in a separate goroutine.
// If a base container is provided it will use it to create the task, otherwise it will create a new container from the image.
func (r *ContainerRuntime) containerCreationRoutine(
	ctx context.Context,
	service model.Service,
	startup chan bool,
	errorchan chan error,
	statusChangeNotificationHandler func(service model.Service),
	creationRoutineOptions ...RoutineOption,
) {

	routineConfig := RoutineOptionConfig{}
	for _, opt := range creationRoutineOptions {
		opt(&routineConfig)
	}

	// if a temporary directory is provided, make sure we clean it up after the routine is done
	if routineConfig.withImageStatePath != "" {
		defer func() {
			if err := os.RemoveAll(routineConfig.withImageStatePath); err != nil {
				logger.ErrorLogger().Printf("Unable to remove temporary directory %s: %v", routineConfig.withImageStatePath, err)
			}
		}()
	}

	// Get or generate the task
	currentContainer := &ContainerService{
		Service: service,
	}
	taskid := genTaskID(service.Sname, service.Instance)
	if s, exists := r.services[taskid]; !exists || s == nil {
		r.services[taskid] = currentContainer
	} else {
		// container exists already, this can only happen during migration
		// check if status is MIGRATION_PROGRESS
		if currentContainer.Status != model.SERVICE_MIGRATION_PROGRESS {
			logger.ErrorLogger().Printf("Service %s instance %d is already deployed, cannot create a new task", service.Sname, service.Instance)
			errorchan <- fmt.Errorf("service %s instance %d is already deployed", service.Sname, service.Instance)
			return
		}
		currentContainer = s
	}

	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		currentContainer.Status = model.SERVICE_FAILED
		delete(r.services, taskid)
		defer statusChangeNotificationHandler(currentContainer.Service)
		if currentContainer.container != nil {
			defer r.removeContainer(currentContainer.container)
		}
	}

	// create base container for this service, if not provided
	container := routineConfig.withContainer
	if container == nil {
		if routineConfig.withImage == nil {
			logger.ErrorLogger().Printf("ERROR: no image provided for service %s instance %d", service.Sname, service.Instance)
			revert(fmt.Errorf("no image provided"))
			return
		}
		createdContainer, err := r.createContainer(ctx, taskid, service, routineConfig.withImage, []containerd.NewContainerOpts{})
		if err != nil {
			logger.ErrorLogger().Printf("ERROR: containerd container creation failure: %v", err)
			revert(err)
			return
		}
		container = createdContainer
	}
	currentContainer.container = container

	//create /tmp/taskid as default log directory
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

	// if a state path is provided, use it to restore the image's task
	newTaskOpts := []containerd.NewTaskOpts{}
	if routineConfig.withImageStatePath != "" {
		newTaskOpts = append(newTaskOpts, containerd.WithRestoreImagePath(routineConfig.withImageStatePath))
		defer func() {
			if err := os.Remove(routineConfig.withImageStatePath); err != nil {
				logger.ErrorLogger().Printf("Unable to remove state file %s: %v", routineConfig.withImageStatePath, err)
			}
		}()
	}

	// startup or resume the container task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStreams(nil, file, file)), newTaskOpts...)
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
	r.channelLock.Lock()
	currentContainer.Status = model.SERVICE_RUNNING
	r.channelLock.Unlock()

	// wait for manual task kill or task finish
	exitStatus := <-exitStatusC
	exitCode := exitStatus.ExitCode()

	r.channelLock.Lock()
	if exitCode == 0 && service.OneShot && currentContainer.Status == model.SERVICE_RUNNING {
		currentContainer.Status = model.SERVICE_COMPLETED
	}
	if exitCode != 0 && currentContainer.Status == model.SERVICE_RUNNING {
		currentContainer.Status = model.SERVICE_FAILED
	}
	if exitCode == 0 && currentContainer.Status != model.SERVICE_MIGRATION_PROGRESS {
		currentContainer.Status = model.SERVICE_DEAD
	}
	r.channelLock.Unlock()

	//detaching network
	if model.GetNodeInfo().Overlay {
		_ = requests.DetachNetworkFromTask(service.Sname, service.Instance)
	}

	_ = r.Undeploy(service.Sname, service.Instance)
	statusChangeNotificationHandler(currentContainer.Service)
}

// createContainer creates a new container for the given service
// It sets the container options based on the service configuration and returns the created container.
func (r *ContainerRuntime) createContainer(ctx context.Context, taskid string, service model.Service, image containerd.Image, containerOpts []containerd.NewContainerOpts) (containerd.Container, error) {

	// generate hostname
	hostname := fmt.Sprintf("instance-%d", service.Instance)

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
	if image != nil {
		containerOpts = append(containerOpts, containerd.WithImage(image))
	}

	// ---- Custom container general oci specs
	specOpts := []oci.SpecOpts{
		oci.WithHostHostsFile,
		oci.WithHostname(hostname),
		oci.WithEnv(append([]string{fmt.Sprintf("HOSTNAME=%s", hostname)}, service.Env...)),
	}
	if image != nil {
		specOpts = append(specOpts, oci.WithImageConfig(image))
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
		return nil, err
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
		return nil, err
	}

	return container, nil
}

// getContainerImage retrieves the container image specified in the service from the containerd client
func (r *ContainerRuntime) getContainerImage(service model.Service) (containerd.Image, error) {
	var image containerd.Image

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
					return nil, err
				}
			} else {
				return nil, err
			}
		}
	}
	return image, nil
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
		for range time.Tick(every) {
			deployedContainers, err := r.containerClient.Containers(r.ctx)
			if err != nil {
				logger.ErrorLogger().Printf("Unable to fetch running containers: %v", err)
			}

			resourceList := make([]model.Resources, 0)

			for _, container := range deployedContainers {
				task, err := container.Task(r.ctx, nil)
				if err != nil {
					//check if migration in progess for this container
					r.channelLock.Lock()
					service, exists := r.services[container.ID()]
					r.channelLock.Unlock()
					if exists && service != nil {
						if service.Status == model.SERVICE_MIGRATION_PROGRESS {
							logger.InfoLogger().Printf("Container %s is in migration progress, skipping resource monitoring", container.ID())
						}
					} else {
						logger.ErrorLogger().Printf("Unable to fetch container task: %v", err)
						err := r.removeContainer(container)
						if err != nil {
							logger.ErrorLogger().Printf("Unable to remove container %s: %v", container.ID(), err)
						}
					}
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

				//Get service status.
				r.channelLock.RLock()
				currentService := r.services[container.ID()]
				r.channelLock.RUnlock()
				status := model.SERVICE_DEAD
				if currentService != nil {
					status = currentService.Status
				}

				resourceList = append(resourceList, model.Resources{
					Cpu:      fmt.Sprintf("%f", cpuUsage),
					Memory:   fmt.Sprintf("%f", mem),
					Disk:     fmt.Sprintf("%d", usage.Size),
					Sname:    extractSnameFromTaskID(container.ID()),
					Runtime:  string(model.CONTAINER_RUNTIME),
					Logs:     getLogs(container.ID()),
					Instance: extractInstanceNumberFromTaskID(container.ID()),
					Status:   status,
				})
			}
			//NOTIFY WITH THE CURRENT CONTAINERS STATUS
			notifyHandler(resourceList)
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
	if container == nil {
		return nil
	}

	logger.InfoLogger().Printf("Clenaning up container: %s", container.ID())
	containerMetadata, err := container.Info(r.ctx)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to fetch container metadata: %v", err)
	}

	//kill task
	task, err := container.Task(r.ctx, nil)
	if err == nil {
		err := killTask(r.ctx, task, container)
		if err != nil {
			logger.ErrorLogger().Printf("Unable to kill task: %v", err)
		}
	}

	//remove snapshotter
	if containerMetadata.Snapshotter != "" {
		currentsnapshotter := r.containerClient.SnapshotService(containerMetadata.Snapshotter)
		if currentsnapshotter != nil {
			err = currentsnapshotter.Remove(r.ctx, containerMetadata.SnapshotKey)
			if err != nil {
				logger.ErrorLogger().Printf("Unable to remove snapshotter %s: %v", containerMetadata.Snapshotter, err)
			}
			currentsnapshotter.Close()
		}
	}

	// remove container
	err = container.Delete(r.ctx)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to delete container: %v", err)
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

// -------- Container migration implementation

// SetMigrationCandidate checks if the service can be migrated and marks it as a candidate.
func (r *ContainerRuntime) SetMigrationCandidate(sname string, instance int) (model.Service, error) {
	taskid := genTaskID(sname, instance)

	r.channelLock.Lock()
	service, exists := r.services[taskid]
	r.channelLock.Unlock()

	if !exists || service == nil {
		return model.Service{}, fmt.Errorf("service %s instance %d is not deployed", sname, instance)
	}

	// check if the service is in any of the migration statuses
	if service.Status == model.SERVICE_MIGRATION_ACCEPTED ||
		service.Status == model.SERVICE_MIGRATION_PROGRESS ||
		service.Status == model.SERVICE_MIGRATION_REQUESTED ||
		service.Status == model.SERVICE_MIGRATION_DEBOUNCE {
		return model.Service{}, fmt.Errorf("service %s instance %d is already in migration process", sname, instance)
	}

	// check if service is running
	if service.Status != model.SERVICE_RUNNING {
		return model.Service{}, fmt.Errorf("service %s instance %d is not running, cannot mark as migration candidate", sname, instance)
	}

	// mark the service as a migration candidate
	r.channelLock.Lock()
	service.Status = model.SERVICE_MIGRATION_ACCEPTED
	r.channelLock.Unlock()

	return service.Service, nil
}

func (r *ContainerRuntime) RemoveMigrationCandidate(sname string, instance int) error {
	taskid := genTaskID(sname, instance)

	r.channelLock.Lock()
	service, exists := r.services[taskid]
	r.channelLock.Unlock()

	if !exists || service == nil {
		return fmt.Errorf("service %s instance %d is not deployed", sname, instance)
	}

	// check if the service is in any of the migration statuses
	if service.Status == model.SERVICE_MIGRATION_ACCEPTED {
		r.channelLock.Lock()
		service.Status = model.SERVICE_RUNNING
		r.channelLock.Unlock()
	} else {
		return fmt.Errorf("service %s instance %d is not marked as a migration candidate", sname, instance)
	}

	return nil
}

// StopAndGetState stops a service and returns its the state file if it has been marked as a migration candidate.
func (r *ContainerRuntime) StopAndGetState(sname string, instance int) (utils.OnceReader, error) {
	taskid := genTaskID(sname, instance)

	r.channelLock.Lock()
	service, exists := r.services[taskid]
	r.channelLock.Unlock()

	if !exists || service == nil {
		return nil, fmt.Errorf("service %s instance %d is not deployed", sname, instance)
	}

	// check if the service is in any of the migration statuses
	if service.Status != model.SERVICE_MIGRATION_ACCEPTED {
		return nil, fmt.Errorf("service %s instance %d is not marked as a migration candidate", sname, instance)
	}

	r.channelLock.Lock()
	service.Status = model.SERVICE_MIGRATION_PROGRESS
	r.channelLock.Unlock()

	revertState := func() {
		r.channelLock.Lock()
		service.Status = model.SERVICE_RUNNING
		r.channelLock.Unlock()
	}

	// stop the task and get its state
	task, err := service.container.Task(r.ctx, nil)
	if err != nil {
		defer revertState()
		return nil, err
	}

	revertRunning := func() {
		task.Resume(r.ctx)
	}

	err = task.Pause(r.ctx)
	if err != nil {
		defer revertState()
		defer revertRunning()
		return nil, err
	}

	stateDir := fmt.Sprintf("%s/%s.checkpoint", model.GetNodeInfo().CheckpointDirectory, taskid)
	stateFile := fmt.Sprintf("%s/%s.checkpoint.tar.gz", model.GetNodeInfo().CheckpointDirectory, taskid)
	err = os.MkdirAll(stateDir, 0755)
	if err != nil {
		defer revertState()
		defer revertRunning()
		logger.ErrorLogger().Printf("Unable to create checkpoint directory: %v", err)
		return nil, fmt.Errorf("unable to create checkpoint directory: %v", err)
	}

	_, err = task.Checkpoint(r.ctx, containerd.WithCheckpointImagePath(stateDir))
	if err != nil {
		defer revertState()
		defer revertRunning()
		logger.ErrorLogger().Printf("Unable to checkpoint the task: %v", err)
		return nil, fmt.Errorf("unable to checkpoint the task: %v", err)
	}

	// compress stateDir into stateFile.tar.gz
	cmd := exec.Command("tar", "-czf", stateFile, "-C", stateDir, ".")
	if err := cmd.Run(); err != nil {
		defer revertState()
		defer revertRunning()
		logger.ErrorLogger().Printf("Unable to compress checkpoint directory: %v", err)
		return nil, fmt.Errorf("unable to compress checkpoint directory: %v", err)
	}

	// read the compressed file into a byte slice
	f, err := os.Open(stateFile)
	if err != nil {
		defer revertState()
		defer revertRunning()
		logger.ErrorLogger().Printf("Unable to read compressed checkpoint file: %v", err)
		return nil, fmt.Errorf("unable to read compressed checkpoint file: %v", err)
	}
	reader := utils.NewOnceReader(f)

	// remove the state directory
	os.RemoveAll(stateDir)

	// let's undeploy the service after migration
	go r.Undeploy(service.Sname, service.Instance)

	return reader, nil
}

// PrepareForInstantiantion prepares the service for instantiation after migration.
func (r *ContainerRuntime) PrepareForInstantiantion(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {

	taskid := genTaskID(service.Sname, service.Instance)

	// check if the service is already deployed
	_, err := r.getContainerByTaskID(taskid)
	if err == nil {
		return fmt.Errorf("task already deployed")
	}
	r.channelLock.Lock()
	val, exists := r.services[taskid]
	r.channelLock.Unlock()
	if exists && val != nil {
		return fmt.Errorf("service %s instance %d is already deployed", service.Sname, service.Instance)
	}

	// Add the application to the services map
	containerService := ContainerService{
		Service: service,
	}
	r.channelLock.Lock()
	r.services[taskid] = &containerService
	r.channelLock.Unlock()

	// Update the service status to migration requested
	r.channelLock.Lock()
	containerService.Status = model.SERVICE_MIGRATION_PROGRESS
	r.channelLock.Unlock()

	// Notify the status change to cluster orchestrator
	statusChangeNotificationHandler(service)

	// Get container image
	image, err := r.containerClient.GetImage(r.ctx, service.Image)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to get image %s for service %s instance %d: %v", service.Image, service.Sname, service.Instance, err)
		r.channelLock.Lock()
		service.Status = model.SERVICE_DEAD
		r.channelLock.Unlock()
		defer statusChangeNotificationHandler(service)
		return fmt.Errorf("unable to get image %s for service %s instance %d: %v", service.Image, service.Sname, service.Instance, err)
	}

	// Create a new container for the service
	container, err := r.createContainer(r.ctx, taskid, service, nil, []containerd.NewContainerOpts{containerd.WithNewSpec(oci.WithImageConfig(image))})
	if err != nil {
		logger.ErrorLogger().Printf("Unable to create container for service %s instance %d: %v", service.Sname, service.Instance, err)
		r.channelLock.Lock()
		service.Status = model.SERVICE_DEAD
		r.channelLock.Unlock()
		defer statusChangeNotificationHandler(service)
		return fmt.Errorf("unable to create container for service %s instance %d: %v", service.Sname, service.Instance, err)
	}
	containerService.container = container

	return nil
}

func (r *ContainerRuntime) AbortMigration(service model.Service) error {

	taskid := genTaskID(service.Sname, service.Instance)

	// check if the service is already deployed
	r.channelLock.Lock()
	s, exists := r.services[taskid]
	r.channelLock.Unlock()
	if !exists || s == nil {
		return fmt.Errorf("service %s instance %d is already undeployed", service.Sname, service.Instance)
	}

	if s.Status != model.SERVICE_MIGRATION_PROGRESS {
		return fmt.Errorf("service %s instance %d is not in migration progress", service.Sname, service.Instance)
	}

	r.Undeploy(service.Sname, service.Instance)

	return nil
}

// ResumeFromState resumes a service prepared for instantiation with a given state.
func (r *ContainerRuntime) ResumeFromState(sname string, instance int, stateFile string, statusChangeNotificationHandler func(service model.Service)) error {

	// remove the state file after the function execution
	defer func() {
		if err := os.Remove(stateFile); err != nil {
			logger.ErrorLogger().Printf("Unable to remove state file %s: %v", stateFile, err)
		}
	}()

	revert := func() {
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		service, exists := r.services[genTaskID(sname, instance)]
		if exists && service != nil {
			service.Status = model.SERVICE_DEAD
			statusChangeNotificationHandler(service.Service)
		}
	}

	taskid := genTaskID(sname, instance)

	r.channelLock.Lock()
	service, exists := r.services[taskid]
	r.channelLock.Unlock()
	if !exists || service == nil {
		return fmt.Errorf("service %s instance %d is not deployed", sname, instance)
	}

	// check if the service is in migration progress
	if service.Status != model.SERVICE_MIGRATION_PROGRESS {
		revert()
		return fmt.Errorf("service %s instance %d is not in migration progress", sname, instance)
	}

	// Check if service container exists
	if service.container == nil {
		revert()
		return fmt.Errorf("service %s instance %d container is not available", sname, instance)
	}

	// Create temporary directory for the state
	stateDir := fmt.Sprintf("%s/%s.checkpoint", model.GetNodeInfo().CheckpointDirectory, taskid)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		logger.ErrorLogger().Printf("Unable to create temporary directory %s: %v", stateDir, err)
		revert()
		return fmt.Errorf("unable to create temporary directory %s: %v", stateDir, err)
	}

	// Uncompress the state file to a temporary directory
	cmd := exec.Command("tar", "-xzf", stateFile, "-C", stateDir)
	if err := cmd.Run(); err != nil {
		logger.ErrorLogger().Printf("Unable to uncompress state file %s: %v", stateFile, err)
		revert()
		return fmt.Errorf("unable to uncompress state file %s: %v", stateFile, err)
	}

	// create startup routine which will accompany the container through its lifetime
	startupChannel := make(chan bool)
	errorChannel := make(chan error)
	go r.containerCreationRoutine(
		r.ctx,
		service.Service,
		startupChannel,
		errorChannel,
		statusChangeNotificationHandler,
		WithContainer(service.container),
		WithImageStatePath(stateDir),
	)

	// wait for updates regarding the container creation
	if !<-startupChannel {
		return <-errorChannel
	}

	return nil
}

// WithContainer sets the container to be used in the routine creation
// If not set, a new container will be created from the image.
func WithContainer(container containerd.Container) RoutineOption {
	return func(c *RoutineOptionConfig) {
		c.withContainer = container
	}
}

// WithImageStatePath sets the path to the image state file to be used in the routine creation
// If not set, the task will be created from the image without restoring any state.
func WithImageStatePath(path string) RoutineOption {
	return func(c *RoutineOptionConfig) {
		c.withImageStatePath = path
	}
}

// WithImage sets the image to be used in the routine creation
func WithImage(image containerd.Image) RoutineOption {
	return func(c *RoutineOptionConfig) {
		c.withImage = image // set the image to be used in the routine creation
	}
}
