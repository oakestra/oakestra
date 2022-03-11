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
	"github.com/opencontainers/runtime-spec/specs-go"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"io/ioutil"
	"os"
	"reflect"
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

var once sync.Once

const NAMESPACE = "edge.io"

func GetContainerdClient() *ContainerRuntime {
	once.Do(func() {
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

func (r *ContainerRuntime) Deploy(service model.Service) error {

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
		service.Sname,
		service.Commands,
		fmt.Sprintf("%d", service.Port),
		startupChannel,
		errorChannel,
		&killChannel,
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
	sname string,
	cmd []string,
	port string,
	startup chan bool,
	errorchan chan error,
	killChannel *chan bool,
) {

	revert := func(err error) {
		startup <- false
		errorchan <- err
	}
	//create container general oci specs
	hostname := fmt.Sprintf("%s.instance", sname)
	specs := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostHostsFile,
		oci.WithHostname(hostname),
		oci.WithEnv([]string{fmt.Sprintf("HOSTNAME=%s", hostname)}),
	}
	//add user defined commands
	if len(cmd) > 0 {
		specs = append(specs, oci.WithProcessArgs(cmd...))
	}
	//add resolve file with default google dns
	resolvconfFile, err := getGoogleDNSResolveConf()
	if err != nil {
		revert(err)
		return
	}
	defer resolvconfFile.Close()
	_ = resolvconfFile.Chmod(444)
	specs = append(specs, withCustomResolvConf(resolvconfFile.Name()))

	// create the container
	container, err := r.contaierClient.NewContainer(
		ctx,
		sname,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(fmt.Sprintf("%s-snapshotter", sname), image),
		containerd.WithNewSpec(specs...),
	)
	if err != nil {
		revert(err)
		return
	}
	defer container.Delete(ctx)

	//	start task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task creation failure: %v", err)
		revert(err)
		return
	}
	defer task.Delete(ctx)

	// if Overlay mode is active then attach network to the task
	if model.GetNodeInfo().Overlay {
		taskpid := int(task.Pid())
		err = requests.AttachNetworkToTask(taskpid, sname, 0)
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
		_ = requests.DetachNetworkFromTask(sname, 0)
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
		container.Delete(ctx)
		if err == nil {
			logger.ErrorLogger().Printf("Task %s terminated", sname)
			*killChannel <- true
		}
	}

	// removing self from kill queue
	r.channelLock.Lock()
	defer r.channelLock.Unlock()
	r.killQueue[sname] = nil
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
