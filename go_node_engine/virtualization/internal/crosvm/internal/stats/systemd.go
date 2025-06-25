package stats

import (
	"encoding/hex"
	"fmt"
	"github.com/containers/storage/pkg/reexec"
	"github.com/godbus/dbus/v5"
	"github.com/prometheus/procfs"
	"go_node_engine/util/iotools"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

func init() {
	reexec.Register("cgroup-run", cgroupRun)
}

func cgroupRun() {
	if len(os.Args) < 4 {
		panic("expected at least 4 arguments")
	}

	client, err := NewSystemdClient()
	if err != nil {
		panic(err)
	}

	machineName := os.Args[1]
	machineType := os.Args[2]

	machine, err := client.CreateMachine(machineName, os.Getpid(), MachineType(machineType))
	if err != nil {
		panic(err)
	}

	subCgroupPath := path.Join(machine.CgroupPath, "instance")
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

	parentCgroupControllersPath := path.Join(machine.CgroupPath, "cgroup.controllers")
	parentCgroupControllersFile, err := os.OpenFile(parentCgroupControllersPath, os.O_RDONLY, 0)
	if err != nil {
		panic(err)
	}
	parentCgroupControllersBytes, err := io.ReadAll(parentCgroupControllersFile)
	if err != nil {
		panic(err)
	}
	iotools.CloseOrWarn(parentCgroupControllersFile, parentCgroupControllersPath)

	// TODO(axiphi): add all to subtree_control
	_ = strings.Split(string(parentCgroupControllersBytes), " ")

	parentCgroupSubtreeControlPath := path.Join(machine.CgroupPath, "cgroup.subtree_control")
	parentCgroupSubtreeControlFile, err := os.OpenFile(parentCgroupControllersPath, os.O_WRONLY, 0)
	if err != nil {
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

type SystemdClient struct {
	conn        *dbus.Conn
	machineRoot dbus.BusObject
}

type MachineInfo struct {
	CgroupPath string
}

func NewSystemdClient() (*SystemdClient, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}

	return &SystemdClient{
		conn:        conn,
		machineRoot: conn.Object("org.freedesktop.machine1", "/org/freedesktop/machine1"),
	}, nil
}

type MachineType string

const (
	MachineTypeVM        MachineType = "vm"
	MachineTypeContainer MachineType = "container"
)

const (
	serviceName = "oakestra"
)

func (c *SystemdClient) CreateMachine(name string, pid int, machineType MachineType) (*MachineInfo, error) {
	type SystemdProperty struct {
		Key   string
		Value interface{}
	}

	var machinePath dbus.ObjectPath
	if err := c.machineRoot.Call(
		"org.freedesktop.machine1.Manager.CreateMachine",
		0,
		name,
		[]byte{},
		// The service name that owns the machine; it can only contain alphanumeric characters and ".".
		// (strings containing "-" also work initially but break when determining the cgroup of the machine)
		serviceName,
		machineType,
		uint32(pid),
		"",
		[]SystemdProperty{
			{
				Key:   "CPUAccounting",
				Value: true,
			},
			{
				Key:   "MemoryAccounting",
				Value: true,
			},
			{
				Key:   "DelegateSubgroup",
				Value: "instance",
			},
		},
	).Store(&machinePath); err != nil {
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}

	machine := c.conn.Object("org.freedesktop.machine1", machinePath)
	unitProp, err := machine.GetProperty("org.freedesktop.machine1.Machine.Unit")
	if err != nil {
		return nil, fmt.Errorf("failed to get machine unit name: %w", err)
	}

	unitPath, ok := unitProp.Value().(string)
	if !ok {
		return nil, fmt.Errorf("got invalid machine unit name: %v", unitProp.Value())
	}

	safeUnitPath := escapeObjectPath(unescapeSystemdName(unitPath))
	unit := c.conn.Object("org.freedesktop.systemd1", dbus.ObjectPath("/org/freedesktop/systemd1/unit/"+safeUnitPath))

	cgroupProp, err := unit.GetProperty("org.freedesktop.systemd1.Scope.ControlGroup")
	if err != nil {
		return nil, fmt.Errorf("failed to get unit cgroup: %w", err)
	}

	cgroup, ok := cgroupProp.Value().(string)
	if !ok || cgroup == "" {
		return nil, fmt.Errorf("got invalid cgroup: %v", cgroupProp.Value())
	}

	return &MachineInfo{
		CgroupPath: path.Join("/sys/fs/cgroup", cgroup),
	}, nil
}

func (c *SystemdClient) Close() error {
	return c.conn.Close()
}

var systemdEscapeSequence = regexp.MustCompile(`(?m)\\x[A-Fa-f0-9]{2}`)

func unescapeSystemdName(systemdName string) string {
	// Systemd gives us escaped unit names, that we need to un-escape.
	// Generally unit names contain only ASCII letters, digits, ":", "-", "_", ".", and "\".
	//
	// Escaped unit names have the following rules:
	// - "/" is replaced by "-"
	// - characters which are not ASCII alphanumerics, ":", "_" or "." are replaced by C-style "\x2d" escapes
	// - "." is replaced with such a C-style escape when it's the first character in the escaped string
	//
	// To un-escape such an escaped unit name, we just need to look for matches of the regexp "\\x[A-Fa-f0-9]{2}",
	// since literal backslashes don't appear in the string as-is (they are also escaped with a \x5c sequence).
	//
	// We don't want to un-do the "/" to "-" replacing, because systemd actually requires that character to stay
	// replaced in its dbus calls.

	return systemdEscapeSequence.ReplaceAllStringFunc(systemdName, func(s string) string {
		hexChars := s[2:]
		hexBytes, err := hex.DecodeString(hexChars)
		if err != nil {
			// can never fail because the regex ensures only valid strings occur here
			panic(err)
		}

		return string(hexBytes)
	})
}

var nonAlphanumeric = regexp.MustCompile("[^a-zA-Z0-9]")

// escapeObjectPath escapes the given string so that it is a valid object path to be used with dbus.
// The path argument must only contain ASCII characters or this function will not work correctly.
func escapeObjectPath(path string) string {
	return nonAlphanumeric.ReplaceAllStringFunc(path, func(s string) string {
		return "_" + hex.EncodeToString([]byte(s))
	})
}

func RetrievePidCgroupPath(pid int) (string, error) {
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return "", err
	}

	proc, err := fs.Proc(pid)
	if err != nil {
		return "", err
	}

	cgroups, err := proc.Cgroups()
	if err != nil {
		return "", err
	}

	if len(cgroups) != 1 {
		return "", fmt.Errorf("the specified process with pid %d is not part of a cgroup v2 hierarchy", pid)
	}

	return path.Join("/sys/fs/cgroup", cgroups[0].Path), nil
}
