package gpu

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

const (
	bin       = "nvidia-smi"
	queryArg  = "--query-gpu="
	formatArg = "--format=csv,noheader,nounits"
)

const allGpusQuery = "index,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,driver_version"

// ParseQueryAll indexes rows positionally, so its row width must follow the query.
var allGpusFields = strings.Count(allGpusQuery, ",") + 1

// Info is a single GPU's snapshot. Memory values are MiB, as reported by nvidia-smi.
type Info struct {
	Index         int
	MemTotal      float64
	MemUsed       float64
	MemFree       float64
	Utilization   float64
	Temperature   float64
	DriverVersion string
}

// QueryAll snapshots every GPU with a single nvidia-smi invocation, so one update
// cycle does not spawn a process per GPU per metric.
func QueryAll() ([]Info, error) {
	var out bytes.Buffer

	cmd := exec.Command(bin, queryArg+allGpusQuery, formatArg)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return ParseQueryAll(out.String()), nil
}

// ParseQueryAll parses the csv output of QueryAll. Malformed lines are skipped so a
// single bad row cannot hide the remaining GPUs. Fields nvidia-smi cannot report
// ("[N/A]", "[Not Supported]") become 0 or an empty driver version.
func ParseQueryAll(output string) []Info {
	infos := make([]Info, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) != allGpusFields {
			continue
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		index, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		driver := fields[6]
		if strings.HasPrefix(driver, "[") {
			driver = ""
		}
		infos = append(infos, Info{
			Index:         index,
			MemTotal:      parseNumber(fields[1]),
			MemUsed:       parseNumber(fields[2]),
			MemFree:       parseNumber(fields[3]),
			Utilization:   parseNumber(fields[4]),
			Temperature:   parseNumber(fields[5]),
			DriverVersion: driver,
		})
	}
	return infos
}

func parseNumber(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}
