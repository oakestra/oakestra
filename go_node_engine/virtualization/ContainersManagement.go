package virtualization

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"go_node_engine/logger"
	"go_node_engine/model"
	"sync"
	"time"
)

type ContainerRuntime struct {
	contaierClient *containerd.Client
	killQueue      map[string]*chan bool
	channelLock    *sync.RWMutex
}

var runtime = ContainerRuntime{
	channelLock: &sync.RWMutex{},
}

var once sync.Once

func GetContainerdClient() *ContainerRuntime {
	once.Do(func() {
		client, err := containerd.New("/run/containerd/containerd.sock")
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to start the container engine: %v\n", err)
		}
		runtime.contaierClient = client
		runtime.killQueue = make(map[string]*chan bool)
	})
	return &runtime
}

func (r *ContainerRuntime) StopContainerdClient() {
	for _, channel := range r.killQueue {
		*channel <- true
	}
	time.Sleep(2 * time.Second)
	r.contaierClient.Close()
}

func (r *ContainerRuntime) Deploy(service model.Service) error {
	// create a context with a namespace
	ctx := namespaces.WithNamespace(context.Background(), service.Sname)

	// pull the given image
	image, err := r.contaierClient.Pull(ctx, service.Image, containerd.WithPullUnpack)
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
		ctx,
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

	// create the container
	container, err := r.contaierClient.NewContainer(
		ctx,
		sname,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(fmt.Sprintf("%s-snapshotter", sname), image),
		containerd.WithNewSpec(oci.WithImageConfig(image)),
	)
	if err != nil {
		startup <- false
		errorchan <- err
		return
	}
	defer container.Delete(ctx)

	// attach network
	// TODO

	//	start task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task creation failure: %v", err)
		startup <- false
		errorchan <- err
		return
	}
	defer task.Delete(ctx)

	// get wait channel
	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task wait failure: %v", err)
		startup <- false
		errorchan <- err
		return
	}

	// execute the image's task
	if err := task.Start(ctx); err != nil {
		logger.ErrorLogger().Printf("ERROR: containerd task start failure: %v", err)
		startup <- false
		errorchan <- err
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
		p, err := task.LoadProcess(ctx, task.ID(), nil)
		if err != nil {
			logger.ErrorLogger().Printf("ERROR deleting the task, LoadProcess: %v", err)
		}
		_, err = p.Delete(ctx, containerd.WithProcessKill)
		if err != nil {
			logger.ErrorLogger().Printf("ERROR deleting the task, Delete: %v", err)
		}
		container.Delete(ctx)
		if err == nil {
			logger.ErrorLogger().Printf("Task %s terminated", sname)
		}
	}

	// removing self from kill queue
	r.channelLock.Lock()
	defer r.channelLock.Unlock()
	r.killQueue[sname] = nil
}
