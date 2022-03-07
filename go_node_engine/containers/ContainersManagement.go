package containers

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"go_node_engine/model"
	"log"
	"sync"
	"time"
)

type ContainerRuntime struct {
	contaierClient *containerd.Client
	killQueue      map[string]chan bool
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
			log.Fatalf("Unable to start the container engine: %v\n", err)
		}
		runtime.contaierClient = client
		runtime.killQueue = make(map[string]chan bool)
	})
	return &runtime
}

func (r *ContainerRuntime) StopContainerdClient() {
	for _, channel := range r.killQueue {
		channel <- true
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

	killChannel := make(chan bool, 0)
	startupChannel := make(chan bool, 0)
	errorChannel := make(chan error, 0)

	r.channelLock.RLock()
	if r.killQueue[service.Sname] != nil {
		r.killQueue[service.Sname] = killChannel
	}
	r.channelLock.RUnlock()

	// create startup routine which will accompany the container through its lifetime
	go r.containerCreationRoutine(
		ctx,
		image,
		service.Sname,
		service.Commands,
		fmt.Sprintf("%d", service.Port),
		startupChannel,
		errorChannel,
		killChannel,
	)

	// wait for updates regarding the container creation
	if <-startupChannel != true {
		return <-errorChannel
	}

	return nil
}

func (r *ContainerRuntime) Undeploy(sname string) error {
	r.channelLock.RLock()
	defer r.channelLock.RUnlock()
	if r.killQueue[sname] != nil {
		r.killQueue[sname] <- true
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
	killChannel chan bool,
) {

	// create the container
	container, err := r.contaierClient.NewContainer(
		ctx,
		sname,
		containerd.WithImage(image),
	)
	if err != nil {
		startup <- false
		errorchan <- err
	}
	defer container.Delete(ctx)

	// attach network
	// TODO

	//	start task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		log.Printf("ERROR: containerd deployment failed: %v", err)
		startup <- false
		errorchan <- err
		return
	}
	defer task.Delete(ctx)

	// get wait channel
	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		log.Printf("ERROR: containerd deployment failed: %v", err)
		startup <- false
		errorchan <- err
		return
	}

	// execute the image's task
	if err := task.Start(ctx); err != nil {
		log.Printf("ERROR: containerd deployment failed: %v", err)
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
		log.Printf("WARNING: Container exited %v", exitStatus.Error())
	case <-killChannel:
		task.Delete(ctx)
		log.Printf("Task %s terminated", sname)
	}

	// removing self from kill queue
	r.channelLock.Lock()
	defer r.channelLock.Unlock()
	r.killQueue[sname] = nil
}
