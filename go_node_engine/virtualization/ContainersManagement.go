package virtualization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/containerd/containerd/namespaces"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/process"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ContainerRuntime struct {
	containerClient runtimeapi.RuntimeServiceClient
	imageClient     runtimeapi.ImageServiceClient
	killQueue       map[string]*chan bool
	channelLock     *sync.RWMutex
	ctx             context.Context
}

// maxMsgSize use 16MB as the default message size limit.
// grpc library default is 4MB
const maxMsgSize = 1024 * 1024 * 16
const connectionTimeout = 30 * time.Second

var runtime = ContainerRuntime{
	channelLock: &sync.RWMutex{},
}

var containerSingletonClient sync.Once
var startContainerMonitoring sync.Once

const NAMESPACE = "oakestra"

func GetContainerdClient() *ContainerRuntime {

	runtimeAddress := os.Getenv("OAKESTRA.CONTAINER.RUNTIME")

	//Connect to container and image runtime
	containerSingletonClient.Do(func() {
		var dialOpts []grpc.DialOption
		ctx, _ := context.WithTimeout(context.Background(), connectionTimeout)
		dialOpts = append(dialOpts,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)))

		conn, err := grpc.DialContext(ctx, fmt.Sprintf("unix://%s", runtimeAddress), dialOpts...)
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to start the container engine: %v\n", err)
		}

		runtime.containerClient = runtimeapi.NewRuntimeServiceClient(conn)
		runtime.imageClient = runtimeapi.NewImageServiceClient(conn)

		//verify if connection to runtime works
		if _, err := runtime.containerClient.Version(ctx, &runtimeapi.VersionRequest{}); err != nil {
			logger.ErrorLogger().Fatalf("Connection to container %s runtime failed: %v\n", runtimeAddress, err)
		}

		runtime.killQueue = make(map[string]*chan bool)
		runtime.ctx = namespaces.WithNamespace(context.Background(), NAMESPACE)
		runtime.forceContainerCleanup()
	})
	return &runtime
}

func (r *ContainerRuntime) StopContainerClient() {
	r.channelLock.Lock()
	taskIDs := reflect.ValueOf(r.killQueue).MapKeys()
	r.channelLock.Unlock()

	for _, taskid := range taskIDs {
		err := r.Undeploy(extractSnameFromTaskID(taskid.String()), extractInstanceNumberFromTaskID(taskid.String()))
		if err != nil {
			logger.ErrorLogger().Printf("Unable to undeploy %s, error: %v", taskid.String(), err)
		}
	}
}

func (r *ContainerRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {

	image := runtimeapi.ImageSpec{Image: service.Image}

	// pull the given image
	service.Sandbox = getSandboxConfig(&service)

	_, err := r.imageClient.PullImage(r.ctx, &runtimeapi.PullImageRequest{
		Image:         &image,
		SandboxConfig: service.Sandbox,
	})
	if err != nil {
		return err
	}

	killChannel := make(chan bool, 1)
	startupChannel := make(chan bool)
	errorChannel := make(chan error)

	r.channelLock.RLock()
	el, servicefound := r.killQueue[genTaskID(service.Sname, service.Instance)]
	r.channelLock.RUnlock()
	if !servicefound || el == nil {
		r.channelLock.Lock()
		r.killQueue[genTaskID(service.Sname, service.Instance)] = &killChannel
		r.channelLock.Unlock()
	} else {
		return errors.New("service already deployed")
	}

	// create startup routine which will accompany the container through its lifetime
	go r.containerCreationRoutine(
		r.ctx,
		&image,
		&service,
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
	image *runtimeapi.ImageSpec,
	service *model.Service,
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

	envs := make([]*runtimeapi.KeyValue, 0)
	for _, env := range service.Env {
		keyval := strings.Split(env, "=")
		envs = append(envs, &runtimeapi.KeyValue{
			Key:   keyval[0],
			Value: keyval[1],
		})
	}

	args := make([]string, 0)
	cdidevices := make([]*runtimeapi.CDIDevice, 0)

	//add GPU if needed
	runtimeHandler := ""
	if service.Vgpus > 0 {
		runtimeHandler = "nvidia-container-runtime"
		args = append(args, "nvidia-container-runtime")
		cdidevices = append(cdidevices, &runtimeapi.CDIDevice{
			Name: "nvidia.com/gpu",
		})
		logger.InfoLogger().Printf("NVIDIA - Adding GPU driver")
	}

	sandbox, err := r.containerClient.RunPodSandbox(r.ctx, &runtimeapi.RunPodSandboxRequest{
		Config:         service.Sandbox,
		RuntimeHandler: runtimeHandler,
	})
	if err != nil {
		logger.ErrorLogger().Printf("%v", err)
		return
	}
	service.Sandbox.Metadata.Uid = sandbox.PodSandboxId
	//cleanup sandbox
	defer func() {
		_, _ = r.containerClient.RemovePodSandbox(r.ctx, &runtimeapi.RemovePodSandboxRequest{
			PodSandboxId: sandbox.PodSandboxId,
		})
	}()

	logger.InfoLogger().Printf("Creating Container %s", service.Sname)
	containerResponse, err := r.containerClient.CreateContainer(r.ctx, &runtimeapi.CreateContainerRequest{
		PodSandboxId: sandbox.PodSandboxId,
		Config: &runtimeapi.ContainerConfig{
			Metadata: &runtimeapi.ContainerMetadata{
				Name:    service.Sandbox.GetMetadata().Name,
				Attempt: 0,
			},
			Image:      image,
			Command:    service.Commands,
			Envs:       envs,
			LogPath:    service.Sandbox.LogDirectory,
			Stdin:      false,
			StdinOnce:  false,
			Tty:        false,
			Args:       args,
			CDIDevices: cdidevices,
			Labels:     map[string]string{"oakestra": "oakestra"},
		},
		SandboxConfig: service.Sandbox,
	})
	if err != nil {
		logger.ErrorLogger().Printf("%v", err)
		return
	}
	defer func(ctx context.Context, containerId string) {
		err := r.removeContainer(containerId)
		//removing from killqueue
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[hostname] = nil
		if err != nil {
			if len(*killChannel) < cap(*killChannel) {
				*killChannel <- false
			}
		} else {
			if len(*killChannel) < cap(*killChannel) {
				*killChannel <- true
			}
		}
	}(ctx, containerResponse.GetContainerId())

	taskpid, err := r.getSandboxPid(sandbox.PodSandboxId)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to get container PID: %v", err)
		revert(err)
		return
	}

	// if Overlay mode is active then attach network to the task
	if model.GetNodeInfo().Overlay {
		err = requests.AttachNetworkToTask(taskpid, service.Sname, service.Instance, service.Ports)
		if err != nil {
			logger.ErrorLogger().Printf("Unable to attach network interface to the task: %v", err)
			revert(err)
			return
		}
	}

	// execute the image's task
	logger.InfoLogger().Printf("Starting Container %s", containerResponse.ContainerId)
	_, err = r.containerClient.StartContainer(ctx, &runtimeapi.StartContainerRequest{
		ContainerId: containerResponse.ContainerId,
	})
	if err != nil {
		logger.ErrorLogger().Printf("Unable to get container PID: %v", err)
		revert(err)
		return
	}

	// adv startup finished
	startup <- true

	// detach network when task is dead
	defer func() {
		service.Status = model.SERVICE_DEAD
		//detaching network
		if model.GetNodeInfo().Overlay {
			_ = requests.DetachNetworkFromTask(service.Sname, service.Instance)
		}
		statusChangeNotificationHandler(*service)
		_ = r.removeContainer(containerResponse.ContainerId)

	}()

	// wait for manual task kill or task exit != CONTAINER_RUNNING
	for {
		select {
		//check status every 1 second
		case <-time.NewTimer(time.Second * 5).C:
			//TODO: container exited, do something, notify to cluster manager
			status, err := r.containerClient.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{ContainerId: containerResponse.ContainerId})
			if err != nil {
				logger.ErrorLogger().Printf("%v", err)
				return
			}
			if status.GetStatus().State != runtimeapi.ContainerState_CONTAINER_RUNNING {
				logger.ErrorLogger().Printf("Container Stopped Running, ExitCode: %s", status.GetStatus().ExitCode)
				service.StatusDetail = fmt.Sprintf("Container exited with status: %d", status.GetStatus().ExitCode)
				return
			}
		case <-*killChannel:
			logger.InfoLogger().Printf("Kill channel message received for task %s", containerResponse.ContainerId)
		}
	}
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
				sandboxes, err := r.containerClient.ListPodSandbox(r.ctx, &runtimeapi.ListPodSandboxRequest{})
				if err != nil {
					logger.ErrorLogger().Printf("Unable to fetch running containers: %v", err)
				}

				resourceList := make([]model.Resources, 0)

				for _, sandbox := range sandboxes.Items {
					if sandbox.GetMetadata().Namespace == NAMESPACE {

						stats, err := r.containerClient.PodSandboxStats(r.ctx, &runtimeapi.PodSandboxStatsRequest{
							PodSandboxId: sandbox.Id,
						})
						if err != nil {
							logger.ErrorLogger().Printf("Unable to fetch data for container: %s", sandbox.Metadata.Name)
							continue
						}

						taskpid, err := r.getSandboxPid(sandbox.Id)
						if err != nil {
							if err != nil {
								logger.ErrorLogger().Printf("Unable to fetch pid for container: %s", sandbox.Metadata.Name)
								continue
							}
						}
						cpuUsage, err := getTotalCpuUsageByPid(int32(taskpid))

						resourceList = append(resourceList, model.Resources{
							Cpu:      fmt.Sprintf("%f", cpuUsage),
							Memory:   fmt.Sprintf("%f", float64(stats.GetStats().Linux.Memory.UsageBytes.GetValue())),
							Disk:     fmt.Sprintf("%d", int(stats.GetStats().GetLinux().Containers[0].WritableLayer.GetUsedBytes().GetValue())),
							Sname:    extractSnameFromTaskID(sandbox.Metadata.Name),
							Runtime:  model.CONTAINER_RUNTIME,
							Instance: extractInstanceNumberFromTaskID(sandbox.Metadata.Name),
						})

					}
				}
				//NOTIFY WITH THE CURRENT CONTAINERS STATUS
				notifyHandler(resourceList)
			}
		}
	})
}

func (r *ContainerRuntime) forceContainerCleanup() {
	deployedContainers, _ := r.containerClient.ListContainers(r.ctx, &runtimeapi.ListContainersRequest{
		Filter: &runtimeapi.ContainerFilter{
			LabelSelector: map[string]string{"oakestra": "oakestra"},
		},
	})
	for _, container := range deployedContainers.Containers {
		err := r.removeContainer(container.Id)
		if err != nil {
			logger.ErrorLogger().Printf("%v", err)
		}
	}
	sandboxes, err := r.containerClient.ListPodSandbox(r.ctx, &runtimeapi.ListPodSandboxRequest{})
	if err != nil {
		logger.ErrorLogger().Printf("%v", err)
	}
	for _, sandbox := range sandboxes.Items {
		_, _ = r.containerClient.RemovePodSandbox(r.ctx, &runtimeapi.RemovePodSandboxRequest{
			PodSandboxId: sandbox.Id,
		})
	}
}

func (r *ContainerRuntime) removeContainer(containerid string) error {
	_, _ = r.containerClient.StopContainer(r.ctx, &runtimeapi.StopContainerRequest{
		ContainerId: containerid,
		Timeout:     0,
	})
	_, _ = r.containerClient.RemoveContainer(r.ctx, &runtimeapi.RemoveContainerRequest{
		ContainerId: containerid,
	})
	logger.ErrorLogger().Printf("Task %s terminated", containerid)
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

func getSandboxConfig(service *model.Service) *runtimeapi.PodSandboxConfig {
	taskId := genTaskID(service.Sname, service.Instance)

	//TODO: This runs using default CNI, therefore oakestra net will fail to setup. We need to use oakestra-net as CNI.
	return &runtimeapi.PodSandboxConfig{
		Metadata: &runtimeapi.PodSandboxMetadata{
			Name:      genTaskID(service.Sname, service.Instance),
			Namespace: NAMESPACE,
			Uid:       uuid.New().String(),
		},
		Hostname:     taskId,
		LogDirectory: fmt.Sprintf("/tmp/%s.log", taskId),
		DnsConfig: &runtimeapi.DNSConfig{
			Servers: []string{"8.8.8.8"},
		},
	}

}

func (r *ContainerRuntime) getSandboxPid(sandboxid string) (int, error) {
	sandboxStatus, err := r.containerClient.PodSandboxStatus(r.ctx, &runtimeapi.PodSandboxStatusRequest{
		PodSandboxId: sandboxid,
		Verbose:      true,
	})
	if err != nil {
		return 0, err
	}
	var infoMap map[string]interface{}
	err = json.Unmarshal([]byte(sandboxStatus.Info["info"]), &infoMap)
	if err != nil {
		return 0, err
	}
	pid := int(infoMap["pid"].(float64))
	if err != nil {
		return 0, err
	}
	return pid, nil
}
