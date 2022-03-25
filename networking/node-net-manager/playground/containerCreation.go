package playground

import (
	"context"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"io/ioutil"
)

var client, err = containerd.New("/run/containerd/containerd.sock")
var ctx = namespaces.WithNamespace(context.Background(), "net.p2p.edgeio")

func Start(sname string, imageName string, instance int, cmd []string, iip string, sip string) (*chan bool, string, error) {
	killChan := make(chan bool, 0)
	address := ""

	//pull image
	var image containerd.Image
	// pull the given image
	sysimg, err := client.ImageService().Get(ctx, imageName)
	if err == nil {
		image = containerd.NewImage(client, sysimg)
	} else {
		fmt.Printf("Error retrieving the image: %v \n Trying to pull the image online.", err)

		image, err = client.Pull(ctx, imageName, containerd.WithPullUnpack)
		if err != nil {
			return nil, "", err
		}
	}
	//create container general oci specs
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostHostsFile,
	}
	//add user defined commands
	if len(cmd) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(cmd...))
	}
	//add resolve file with default google dns
	resolvconfFile, _ := ioutil.TempFile("/tmp", "edgeio-resolv-conf")
	_, _ = resolvconfFile.WriteString(fmt.Sprintf("nameserver 8.8.8.8\n"))
	defer resolvconfFile.Close()
	_ = resolvconfFile.Chmod(444)
	specOpts = append(specOpts, withCustomResolvConf(resolvconfFile.Name()))

	// create the container
	container, err := client.NewContainer(
		ctx,
		sname,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(fmt.Sprintf("%s-snapshotter", sname), image),
		containerd.WithNewSpec(specOpts...),
	)
	if err != nil {
		return nil, "", err
	}

	//	start task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return nil, "", err
	}

	// get wait channel
	_, err = task.Wait(ctx)
	if err != nil {
		return nil, "", err
	}

	// if Overlay mode is active then attach network to the task
	address, err = attachNetwork(sname, int(task.Pid()), instance, map[int]int{}, iip, sip)
	if err != nil {
		return nil, "", err
	}

	// execute the image's task
	if err := task.Start(ctx); err != nil {
		return nil, "", err
	}

	go deferredKill(task, container, &killChan)
	return &killChan, address, nil
}

func cleanAll() {
	deployedContainers, _ := client.Containers(ctx)
	for _, container := range deployedContainers {
		task, err := container.Task(ctx, nil)
		if err != nil {
			continue
		}
		killTask(task, container)
	}
}

func deferredKill(task containerd.Task, container containerd.Container, kill *chan bool) {
	select {
	case <-*kill:
		killTask(task, container)
	}
}

func killTask(task containerd.Task, container containerd.Container) {
	//removing the task
	p, err := task.LoadProcess(ctx, task.ID(), nil)
	if err != nil {
		return
	}
	_, err = p.Delete(ctx, containerd.WithProcessKill)
	if err != nil {
		return
	}
	_, _ = task.Delete(ctx)
	_ = container.Delete(ctx)
	fmt.Println("task terminated")
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
