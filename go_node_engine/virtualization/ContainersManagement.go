package virtualization

import (
	"context"
	"fmt"
	"go_node_engine/csi"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/model/gpu"
	"go_node_engine/requests"
	virtrt "go_node_engine/virtualization/internal/runtime"
	"iter"
	"maps"
	"os"
	"os/exec"
	"sort"
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
	"github.com/opencontainers/runtime-spec/specs-go"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/shirou/gopsutil/docker"
	"github.com/shirou/gopsutil/process"
	"github.com/struCoder/pidusage"
)

func init() {
	virtrt.Register(string(model.CONTAINER_RUNTIME), newContainerdRuntime)
	plugins := findAdditionalRuntimePlugins()
	if plugins != nil {
		for additionalName := range findAdditionalRuntimePlugins() {
			virtrt.Register(additionalName, newContainerdRuntime)
		}
	}
}

// ContainerRuntime is the struct that describes the container runtime
type ContainerRuntime struct {
	containerClient *containerd.Client
	killQueue       map[string]*chan bool
	// Lock specific for the container runtime interactions.
	// Only one interaction at a time as we have no guarantee the runtime will handle more.
	channelLock *sync.RWMutex
	// mountedVolumes tracks CSI-mounted volumes per task (keyed by taskID).
	// Protected by channelLock.
	mountedVolumes map[string][]csi.MountedVolume
	ctx            context.Context
	wg             sync.WaitGroup
}

// NAMESPACE is the namespace of the runtime
const NAMESPACE = "oakestra"

// CGROUPV1_BASE_MEM is the base memory path for cgroup v1
const CGROUPV1_BASE_MEM = "/sys/fs/cgroup/memory/" + NAMESPACE

// CGROUPV2_BASE_MEM is the base memory path for cgroup v2
const CGROUPV2_BASE_MEM = "/sys/fs/cgroup/" + NAMESPACE

// Containerd config path
const CONTAINERD_CONFIG_PATH = "/etc/containerd/config.toml"

// Max container cleanup duration
const CLEANUP_TIMEOUT = 5 * time.Second

// GetContainerdRuntime returns the container runtime client
func newContainerdRuntime(_ virtrt.RuntimeInfo) virtrt.Runtime {
	client, err := containerd.New("/run/containerd/containerd.sock")
	if err != nil {
		logger.ErrorLogger().Printf("Unable to start the container engine: %v\n", err)
		return &ContainerRuntime{}
	}

	runtime := ContainerRuntime{
		channelLock: &sync.RWMutex{},
	}
	runtime.containerClient = client
	runtime.killQueue = make(map[string]*chan bool)
	runtime.mountedVolumes = make(map[string][]csi.MountedVolume)
	runtime.ctx = namespaces.WithNamespace(context.Background(), NAMESPACE)
	runtime.forceContainerCleanup()
	runtime.mountedVolumes = make(map[string][]csi.MountedVolume)

	// Advertise sub-runtimes discovered from the containerd config (e.g. runc).
	for name := range findAdditionalRuntimePlugins() {
		model.GetNodeInfo().AddSupportedTechnology(model.RuntimeType(name))
	}

	return &runtime
}

// checks the containerd config file for additional runtimes and registers them.
// Parses TOML directly: containerd's v2 LoadConfig rejects legacy short-form
// disabled_plugins entries (e.g. "cri") that ship with Docker's containerd.
func findAdditionalRuntimePlugins() iter.Seq[string] {
	data, err := os.ReadFile(CONTAINERD_CONFIG_PATH)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to read containerd config file: %v", err)
		return nil
	}
	var containerdConfig struct {
		Plugins map[string]interface{} `toml:"plugins"`
	}
	if err := toml.Unmarshal(data, &containerdConfig); err != nil {
		logger.ErrorLogger().Printf("Unable to parse containerd config file: %v", err)
		return nil
	}
	for _, ctd := range containerdConfig.Plugins {
		ctd, ok := ctd.(map[string]interface{})["containerd"].(map[string]interface{})
		if ok {
			runtimes, ok := ctd["runtimes"].(map[string]interface{})
			if ok {
				for runtimeName := range runtimes {
					logger.InfoLogger().Printf("Adding compatibility custom runtime %s configured in containerd config file %s", runtimeName, CONTAINERD_CONFIG_PATH)
				}
				return maps.Keys(runtimes)
			}
		}
	}
	return nil
}

// StopContainerdClient stops the container runtime client
func (r *ContainerRuntime) Stop() {
	r.channelLock.Lock()
	taskIDs := make([]string, 0, len(r.killQueue))
	for id := range r.killQueue {
		taskIDs = append(taskIDs, id)
	}
	r.channelLock.Unlock()

	for _, taskid := range taskIDs {
		err := r.Undeploy(extractSnameFromTaskID(taskid), extractInstanceNumberFromTaskID(taskid))
		if err != nil {
			logger.ErrorLogger().Printf("Unable to undeploy %s, error: %v", taskid, err)
		}
	}
	waitDone := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		logger.InfoLogger().Printf("All containers stopped cleanly")
	case <-time.After(CLEANUP_TIMEOUT):
		logger.ErrorLogger().Printf("Timed out waiting for containers to stop, forcing cleanup")
		r.forceContainerCleanup()
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

	taskid := genTaskID(service.Sname, service.Instance)
	startupChannel := make(chan bool)
	errorChannel := make(chan error)
	killChannel := make(chan bool, 1)

	r.channelLock.Lock()
	_, alreadyDeployed := r.killQueue[taskid]
	if !alreadyDeployed {
		r.killQueue[taskid] = &killChannel
	}
	r.channelLock.Unlock()

	if alreadyDeployed {
		return fmt.Errorf("task already deployed")
	}

	// create startup routine which will accompany the container through its lifetime
	r.wg.Add(1)
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
	if !<-startupChannel {
		return <-errorChannel
	}

	return nil
}

// Undeploy undeploys a service
func (r *ContainerRuntime) Undeploy(service string, instance int) error {
	taskid := genTaskID(service, instance)
	r.channelLock.RLock()
	ch, found := r.killQueue[taskid]
	r.channelLock.RUnlock()
	if found && ch != nil {
		*ch <- true
		return nil
	}
	logger.ErrorLogger().Printf("Unable to undeploy service %s instance %d: not found", service, instance)
	return fmt.Errorf("service not found")
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

	defer r.wg.Done()

	taskid := genTaskID(service.Sname, service.Instance)
	hostname := fmt.Sprintf("instance-%d", service.Instance)
	service.StatusDetail = "Container is starting up"

	revert := func(err error) {
		service.StatusDetail = fmt.Sprintf("Container failed to start: %v", err)
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		delete(r.killQueue, taskid)
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
		gpuDevices, err := selectBestGPUs(service.Vgpus)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to select GPUs: %v", err)
			// Fallback to GPU 0 if selection fails
			specOpts = append(specOpts, nvidia.WithGPUs(nvidia.WithDevices(0), nvidia.WithAllCapabilities))
			logger.InfoLogger().Printf("NVIDIA - Adding GPU 0 (fallback)")
		} else {
			specOpts = append(specOpts, nvidia.WithGPUs(nvidia.WithDevices(gpuDevices...), nvidia.WithAllCapabilities))
			logger.InfoLogger().Printf("NVIDIA - Adding GPU(s): %v", gpuDevices)
		}
	}
	// ---- mount CSI volumes (stage + publish) and add bind mounts to OCI spec
	var mountedVolumes []csi.MountedVolume
	if len(service.Volumes) > 0 {
		var mountErr error
		mountedVolumes, mountErr = csi.MountVolumes(service)
		if mountErr != nil {
			// Partial mounts are cleaned up before aborting.
			csi.UnmountVolumes(mountedVolumes)
			revert(mountErr)
			return
		}
		specOpts = append(specOpts, withCSIVolumeMounts(mountedVolumes))
		// Store mounted volumes so they can be cleaned up when the container stops.
		r.channelLock.Lock()
		r.mountedVolumes[taskid] = mountedVolumes
		r.channelLock.Unlock()
	}
	// ---- add resolve file with default google dns
	resolvconfFile, err := getResolveConfFile()
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
	var exitStatus containerd.ExitStatus
	select {
	case exitStatus = <-exitStatusC:
		// natural exit, fall through to cleanup below
	case <-*killChannel:
		// kill requested
		service.Status = model.SERVICE_DEAD
		if model.GetNodeInfo().Overlay {
			_ = requests.DetachNetworkFromTask(service.Sname, service.Instance)
		}
		r.channelLock.Lock()
		delete(r.killQueue, taskid)
		r.channelLock.Unlock()
		_ = r.removeContainer(container)
		statusChangeNotificationHandler(service)
		return
	}

	if exitStatus.ExitCode() == 0 && service.OneShot {
		service.Status = model.SERVICE_COMPLETED
	} else {
		service.Status = model.SERVICE_DEAD
	}

	//detaching network
	if model.GetNodeInfo().Overlay {
		_ = requests.DetachNetworkFromTask(service.Sname, service.Instance)
	}

	r.channelLock.Lock()
	delete(r.killQueue, taskid)
	r.channelLock.Unlock()
	_ = r.removeContainer(container)

	// CSI teardown: unpublish + unstage volumes after the container has been removed.
	r.channelLock.Lock()
	mv, hasMounts := r.mountedVolumes[taskid]
	delete(r.mountedVolumes, taskid)
	r.channelLock.Unlock()
	if hasMounts {
		csi.UnmountVolumes(mv)
	}
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
	if r.containerClient == nil {
		return
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for range ticker.C {
		deployedContainers, err := r.containerClient.Containers(r.ctx)
		if err != nil {
			logger.ErrorLogger().Printf("Unable to fetch running containers: %v", err)
		}

		resourceList := make([]model.Resources, 0)

		for _, container := range deployedContainers {
			task, err := container.Task(r.ctx, nil)
			if err != nil {
				logger.ErrorLogger().Printf("Unable to fetch container task: %v", err)

				info, err := container.Info(r.ctx)
				if err != nil {
					logger.ErrorLogger().Printf("Unable to fetch container info: %v", err)
					continue
				}

				// if container created less than 10 seconds ago, then skip removal
				if time.Since(info.CreatedAt) < 10*time.Second {
					logger.InfoLogger().Printf("Skipping container %s, it is still starting up", container.ID())
					continue
				}

				_ = r.removeContainer(container)
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

			memUsage, err := r.getContainerMemoryUsage(container.ID(), int(task.Pid()))
			if err != nil {
				logger.ErrorLogger().Printf("Unable to fetch container Memory: %v", err)
				memUsage = 0
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
				Memory:   fmt.Sprintf("%f", memUsage),
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
		logger.WarnLogger().Printf("Container is nil, nothing to remove")
		return nil
	}

	logger.InfoLogger().Printf("Cleaning up container: %s", container.ID())
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
				logger.WarnLogger().Printf("Unable to remove snapshotter %s: %v", containerMetadata.Snapshotter, err)
			}
			err = currentsnapshotter.Close()
			if err != nil {
				logger.WarnLogger().Printf("Unable to close snapshotter %s: %v", containerMetadata.Snapshotter, err)
			}
		}
	}

	// remove container
	err = container.Delete(r.ctx)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to delete container: %v", err)
		return err
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
	if model.GetNodeInfo().MemoryMB == 0 {
		return 100, nil
	}
	return float64(mem.MemUsageInBytes) / float64(model.GetNodeInfo().MemoryMB*1024*1024), nil
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

// withCSIVolumeMounts returns an OCI SpecOpt that bind-mounts each CSI-published
// volume into the container at the path specified by MountedVolume.MountPath.
// If MountPath is empty the volume is mounted at /mnt/csi/<volumeID>.
func withCSIVolumeMounts(mounts []csi.MountedVolume) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		for _, mv := range mounts {
			dest := mv.MountPath
			if dest == "" {
				dest = "/mnt/csi/" + mv.VolumeID
			}
			s.Mounts = append(s.Mounts, specs.Mount{
				Destination: dest,
				Type:        "bind",
				Source:      mv.TargetPath,
				Options:     []string{"rbind", "rw"},
			})
			logger.InfoLogger().Printf("CSI volume %s bind-mounted %s -> %s", mv.VolumeID, mv.TargetPath, dest)
		}
		return nil
	}
}

func getResolveConfFile() (string, error) {
	//check if /run/systemd/resolve/resolv.conf exists and use that
	if _, err := os.Stat("/run/systemd/resolve/resolv.conf"); err == nil {
		logger.InfoLogger().Printf("Using systemd-resolved resolv.conf")
		return "/run/systemd/resolve/resolv.conf", nil
	}

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

// getContainerByTaskID returns the containerd.Container associated with the given task ID (which is in the format sname.instance.instanceNumber).
/*
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
*/

// gpuInfo holds GPU device information for sorting
type gpuInfo struct {
	index       int
	freeMemory  int64 // in MiB
	totalMemory int64 // in MiB
}

// selectBestGPUs selects up to 'requested' GPUs based on available memory.
// Returns GPU indices sorted by most available memory first.
func selectBestGPUs(requested int) ([]int, error) {
	// Get total number of available GPUs
	totalGPUs, err := gpu.NvsmiDeviceCount()
	if err != nil {
		return nil, fmt.Errorf("failed to query GPU count: %w", err)
	}

	if totalGPUs == 0 {
		return nil, fmt.Errorf("no GPUs available")
	}

	// Cap requested GPUs to available GPUs
	numGPUs := requested
	if numGPUs > totalGPUs {
		logger.WarnLogger().Printf("Requested %d GPUs but only %d available, using %d", requested, totalGPUs, totalGPUs)
		numGPUs = totalGPUs
	}

	// Query memory info for all GPUs
	gpuList := make([]gpuInfo, 0, totalGPUs)
	for i := 0; i < totalGPUs; i++ {
		// Query free memory (memory.free) and total memory (memory.total)
		freeMemStr, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "memory.free")
		if err != nil {
			logger.WarnLogger().Printf("Failed to query free memory for GPU %d: %v", i, err)
			continue
		}

		totalMemStr, err := gpu.NvsmiQuery(fmt.Sprintf("%d", i), "memory.total")
		if err != nil {
			logger.WarnLogger().Printf("Failed to query total memory for GPU %d: %v", i, err)
			continue
		}

		freeMem, err := strconv.ParseInt(freeMemStr, 10, 64)
		if err != nil {
			logger.WarnLogger().Printf("Failed to parse free memory for GPU %d: %v", i, err)
			continue
		}

		totalMem, err := strconv.ParseInt(totalMemStr, 10, 64)
		if err != nil {
			logger.WarnLogger().Printf("Failed to parse total memory for GPU %d: %v", i, err)
			continue
		}

		gpuList = append(gpuList, gpuInfo{
			index:       i,
			freeMemory:  freeMem,
			totalMemory: totalMem,
		})

		logger.InfoLogger().Printf("GPU %d: %d MiB free / %d MiB total", i, freeMem, totalMem)
	}

	if len(gpuList) == 0 {
		return nil, fmt.Errorf("failed to query memory for any GPU")
	}

	// Sort GPUs by free memory (descending - most free memory first)
	sort.Slice(gpuList, func(i, j int) bool {
		return gpuList[i].freeMemory > gpuList[j].freeMemory
	})

	// Select top N GPUs
	selected := make([]int, 0, numGPUs)
	for i := 0; i < numGPUs && i < len(gpuList); i++ {
		selected = append(selected, gpuList[i].index)
	}

	return selected, nil
}
