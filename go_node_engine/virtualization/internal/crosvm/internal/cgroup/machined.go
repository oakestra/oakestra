package cgroup

import (
	"encoding/hex"
	"fmt"
	"github.com/godbus/dbus/v5"
	"path"
	"regexp"
)

type MachinedClient struct {
	conn        *dbus.Conn
	machineRoot dbus.BusObject
}

type MachineInfo struct {
	CgroupPath string
}

func NewSystemdClient() (*MachinedClient, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}

	return &MachinedClient{
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

func (c *MachinedClient) CreateMachine(name string, pid int, machineType MachineType) (*MachineInfo, error) {
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
		// I believe these options technically wouldn't be necessary, as at least CPUAccounting is true by default,
		// but let's be explicit about which features we are using in our systemd scope.
		[]SystemdProperty{
			{
				Key:   "CPUAccounting",
				Value: true,
			},
			{
				Key:   "MemoryAccounting",
				Value: true,
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

func GetMachineCgroupPath(machineName string) string {
	return path.Join("/sys/fs/cgroup/machine.slice/", fmt.Sprintf("machine-%s.scope", machineName), "instance")
}

func (c *MachinedClient) Close() error {
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
