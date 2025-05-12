package fsimg

import "os/exec"

func lookPathOrEmpty(file string) string {
	path, err := exec.LookPath(file)
	if err != nil {
		return ""
	}

	return path
}
