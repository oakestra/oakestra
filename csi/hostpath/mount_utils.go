package main

import (
	"bufio"
	"os"
	"strings"
)

// isMountPoint checks if a given path is an active mount point by reading /proc/mounts.
// This provides a kernel-level verification independent of our tracking.
func isMountPoint(path string) (bool, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return false, err
	}
	defer func() {
		_ = file.Close()
	}()

	// Normalize the path (remove trailing slash for comparison)
	path = strings.TrimRight(path, "/")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		mountPoint := strings.TrimRight(fields[1], "/")
		if mountPoint == path {
			return true, nil
		}
	}

	return false, scanner.Err()
}
