package install

import (
	"os"
	"strings"
)

type OSFamily string

const (
	Apt     OSFamily = "apt"
	Dnf     OSFamily = "dnf"
	Unknown OSFamily = "unknown"
)

func detectOSFamily() OSFamily {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return Unknown
	}
	content := string(b)
	if strings.Contains(content, "ID=ubuntu") || strings.Contains(content, "ID=debian") {
		return Apt
	}
	if strings.Contains(content, "ID=centos") || strings.Contains(content, "ID=fedora") || strings.Contains(content, "ID=rhel") {
		return Dnf
	}
	return Unknown
}
