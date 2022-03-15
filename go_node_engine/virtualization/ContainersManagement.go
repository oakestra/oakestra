package virtualization

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/opencontainers/runtime-spec/specs-go"
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

const NAMESPACE = "edge.io"

func GetContainerdClient() *ContainerRuntime {
	containerdSingletonCLient.Do(func() {
		client, err := containerd.New("/run/containerd/containerd.sock")
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to start the container engine: %v\n", err)
		}
		runtime.contaierClient = client
		runtime.killQueue = make(map[string]*chan bool)
		runtime.ctx = namespaces.WithNamespace(context.Background(), NAMESPACE)
	})
	return &runtime
}

func (r *ContainerRuntime) StopContainerdClient() {
	r.channelLock.Lock()
	servicesNames := reflect.ValueOf(r.killQueue).MapKeys()
	r.channelLock.Unlock()

	for _, sname := range servicesNames {
		err := r.Undeploy(sname.String())
		if err != nil {
			logger.ErrorLogger().Printf("Unable to undeploy %s, error: %v", sname.String(), err)
		}
	}
	r.contaierClient.Close()
}

func (r *ContainerRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {

	// pull the given image
	image, err := r.contaierClient.Pull(r.ctx, service.Image, containerd.WithPullUnpack)
	if err != nil {
		return err
	}

	killChannel := make(chan bool, 1)
	startupChannel := make(chan bool, 0)
	errorChannel := make(chan error, 0)

	r.channelLock.RLock()
	el, servicefound := r.killQueue[service.Sname]
	r.channelLock.RUnlock()
	if !servicefound || el == nil {
		r.channelLock.Lock()
		r.killQueue[service.Sname] = &killChannel
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

func (r *ContainerRuntime) Undeploy(sname string) error {
	r.channelLock.Lock()
	defer r.channelLock.Unlock()
	el, found := r.killQueue[sname]
	if found && el != nil {
		logger.InfoLogger().Printf("Sending kill signal to %s", sname)
		*r.killQueue[sname] <- true
		select {
		case res := <-*r.killQueue[sname]:
			if res == false {
				logger.ErrorLogger().Printf("Unable to stop service %s", sname)
			}
		case <-time.After(5 * time.Second):
			logger.ErrorLogger().Printf("Unable to stop service %s", sname)
		}
		delete(r.killQueue, sname)
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

	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[service.Sname] = nil
	}

	//create container general oci specs
	hostname := fmt.Sprintf("%s.instance", service.Sname)
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostHostsFile,
		oci.WithHostname(hostname),
		oci.WithEnv([]string{fmt.Sprintf("HOSTNAME=%s", hostname)}),
	}
	//add user defined commands
	if len(service.Commands) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(service.Commands...))
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
		service.Sname,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(fmt.Sprintf("%s-snapshotter", service.Sname), image),
		containerd.WithNewSpec(specOpts...),
	)
	if err != nil {
		revert(err)
		return
	}

	//	start task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task creation failure: %v", err)
		_ = container.Delete(ctx)
		revert(err)
		return
	}
	defer func(ctx context.Context, task containerd.Task) {
		_ = killTask(ctx, task, container, killChannel)
		//removing from killqueue
		r.channelLock.Lock()
		defer r.channelLock.Unlock()
		r.killQueue[service.Sname] = nil
	}(ctx, task)

	//create port mappings map
	portMappings, err := createPortMappings(service.Ports)
	if err != nil {
		logger.ErrorLogger().Printf("Invalid port mappings %v", err)
		revert(err)
		return
	}

	// if Overlay mode is active then attach network to the task
	if model.GetNodeInfo().Overlay {
		taskpid := int(task.Pid())
		err = requests.AttachNetworkToTask(taskpid, service.Sname, 0, portMappings)
		if err != nil {
			logger.ErrorLogger().Printf("Unable to attach network interface to the task: %v", err)
			revert(err)
			return
		}
	}

	// get wait channel
	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task wait failure: %v", err)
		revert(err)
		return
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
		logger.InfoLogger().Printf("WARNING: Container exited %v", exitStatus.Error())
	case <-*killChannel:
		logger.InfoLogger().Printf("Kill channel message received for task %s", task.ID())
		//detaching network
		_ = requests.DetachNetworkFromTask(service.Sname, 0)
	}
	service.Status = model.SERVICE_DEAD
	statusChangeNotificationHandler(service)
}

func (r *ContainerRuntime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {
	//start container monitoring service
	startContainerMonitoring.Do(func() {
		for true {
			select {
			case <-time.After(every):
				deployedContainers, err := r.contaierClient.Containers(r.ctx)
				if err != nil {
					logger.ErrorLogger().Printf("Unable to fetch running containers")
				}
				resourceList := make([]model.Resources, 0)
				for _, container := range deployedContainers {
					task, err := container.Task(r.ctx, nil)
					if err != nil {
						logger.ErrorLogger().Printf("Unable to fetch container task")
						continue
					}
					sysInfo, err := pidusage.GetStat(int(task.Pid()))
					if err != nil {
						logger.ErrorLogger().Printf("Unable to fetch task info")
						continue
					}
					_, _, usage, err := storage.GetInfo(r.ctx, fmt.Sprintf("%s-snapshotter", task.ID()))
					if err != nil {
						logger.ErrorLogger().Printf("Unable to fetch task disk usage")
						continue
					}
					resourceList = append(resourceList, model.Resources{
						Cpu:    fmt.Sprintf("%f", sysInfo.CPU),
						Memory: fmt.Sprintf("%f", sysInfo.Memory),
						Disk:   fmt.Sprintf("%d", usage.Size),
						Sname:  task.ID(),
					})
				}
				//NOTIFY WITH THE CURRENT CONTAINERS STATUS
				notifyHandler(resourceList)
			}
		}
	})
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

func killTask(ctx context.Context, task containerd.Task, container containerd.Container, killChannel *chan bool) error {
	//removing the task
	p, err := task.LoadProcess(ctx, task.ID(), nil)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR deleting the task, LoadProcess: %v", err)
		*killChannel <- false
	}
	_, err = p.Delete(ctx, containerd.WithProcessKill)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR deleting the task, Delete: %v", err)
		*killChannel <- false
	}
	_, _ = task.Delete(ctx)
	_ = container.Delete(ctx)

	logger.ErrorLogger().Printf("Task %s terminated", task.ID())
	*killChannel <- true
	return nil
}

func createPortMappings(ports string) (map[int]int, error) {
	portMappings := make(map[int]int, 0)
	mappings := strings.Split(ports, ";")
	for _, portmap := range mappings {
		ports := strings.Split(portmap, ":")
		hostPort, err := strconv.Atoi(ports[0])
		containerPort := hostPort
		if len(ports) > 1 && err == nil {
			containerPort, err = strconv.Atoi(ports[1])
		}
		if err != nil {
			return nil, err
		}
		portMappings[hostPort] = containerPort
	}
	return portMappings, nil
}
