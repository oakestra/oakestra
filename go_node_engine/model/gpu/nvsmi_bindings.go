package gpu

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const (
	bin       = "nvidia-smi"
	gpuArg    = "--id="
	queryArg  = "--query-gpu="
	formatArg = "--format=csv,noheader,nounits"
)

// NvsmiQuery Query to nvidia-smi. Refer to https://nvidia.custhelp.com/app/answers/detail/a_id/3751/~/useful-nvidia-smi-queries
// For the available query
func NvsmiQuery(id string, query string) (string, error) {
	var out bytes.Buffer

	cmd := exec.Command(bin, fmt.Sprintf("%s%s", gpuArg, id), fmt.Sprintf("%s%s", queryArg, query), formatArg)
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// NvsmiDeviceCount Counts the available nvidia-smi compatible GPUs in the machine
func NvsmiDeviceCount() (int, error) {
	var out bytes.Buffer

	query := "gpu_name"
	cmd := exec.Command(bin, fmt.Sprintf("%s%s", queryArg, query), formatArg)
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return 0, err
	}

	nvSmi := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	return len(nvSmi), nil
}
