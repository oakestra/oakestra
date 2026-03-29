package cgroup

import (
	"go_node_engine/util/iotools"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/containers/storage/pkg/reexec"
)

func init() {
	reexec.Register("machined-exec", machinedExec)
}

// machinedExec creates a systemd machine via its dbus API and then moves the current process into the corresponding
// cgroup, then exec'ing specified command.
func machinedExec() {
	if len(os.Args) < 4 {
		panic("expected at least 4 arguments")
	}

	cgroupPath := createMachineFromArgs()

	subCgroupPath := path.Join(cgroupPath, "instance")
	if err := os.Mkdir(subCgroupPath, 0o755); err != nil {
		panic(err)
	}

	subCgroupProcsPath := path.Join(subCgroupPath, "cgroup.procs")
	subCgroupProcsFile, err := os.OpenFile(subCgroupProcsPath, os.O_WRONLY, 0)
	if err != nil {
		panic(err)
	}
	if _, err := subCgroupProcsFile.WriteString(strconv.Itoa(os.Getpid()) + "\n"); err != nil {
		panic(err)
	}
	iotools.CloseOrWarn(subCgroupProcsFile, subCgroupPath)

	log.Printf("Moved pid %d to cgroup %s", os.Getpid(), subCgroupPath)

	parentCgroupControllersPath := path.Join(cgroupPath, "cgroup.controllers")
	parentCgroupControllersFile, err := os.OpenFile(parentCgroupControllersPath, os.O_RDONLY, 0)
	if err != nil {
		panic(err)
	}
	parentCgroupControllersBytes, err := io.ReadAll(parentCgroupControllersFile)
	if err != nil {
		panic(err)
	}
	iotools.CloseOrWarn(parentCgroupControllersFile, parentCgroupControllersPath)

	parentCgroupSubtreeControlPath := path.Join(cgroupPath, "cgroup.subtree_control")
	parentCgroupSubtreeControlFile, err := os.OpenFile(parentCgroupSubtreeControlPath, os.O_WRONLY, 0)
	if err != nil {
		panic(err)
	}
	parentCgroupSubtreeControlCommand := convertControllersToSubtreeControlCommand(string(parentCgroupControllersBytes))
	if _, err := parentCgroupSubtreeControlFile.WriteString(parentCgroupSubtreeControlCommand); err != nil {
		panic(err)
	}
	iotools.CloseOrWarn(parentCgroupSubtreeControlFile, parentCgroupSubtreeControlPath)

	execBin, err := exec.LookPath(os.Args[3])
	if err != nil {
		panic(err)
	}
	execArgs := os.Args[3:]

	log.Printf("Running exec: %s %v", execBin, execArgs)

	if err := syscall.Exec(execBin, execArgs, os.Environ()); err != nil {
		panic(err)
	}

	log.Printf("Exec seems to have failed")
}

func createMachineFromArgs() string {
	client, err := NewSystemdClient()
	if err != nil {
		panic(err)
	}
	defer iotools.CloseOrWarn(client, "SystemdClient")

	machineName := os.Args[1]
	machineType := os.Args[2]

	machine, err := client.CreateMachine(machineName, os.Getpid(), MachineType(machineType))
	if err != nil {
		panic(err)
	}

	return machine.CgroupPath
}

func convertControllersToSubtreeControlCommand(controllers string) string {
	controllerElements := strings.Split(controllers, " ")

	var controllerCommands []string
	for _, controller := range controllerElements {
		controllerCommands = append(controllerCommands, "+"+controller)
	}

	return strings.Join(controllerCommands, " ") + "\n"
}

func MachinedExecCommand(machineName string, machineType MachineType, args ...string) *exec.Cmd {
	return reexec.Command(slices.Concat(
		[]string{"machined-exec", machineName, string(machineType)},
		args,
	)...)
}
