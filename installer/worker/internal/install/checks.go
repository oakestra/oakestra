package install

import (
	"errors"
	"os"
	"runtime"
)

func checkPrerequisites() error {
	if runtime.GOOS != "linux" {
		return errors.New("the worker installer requires Linux")
	}
	// sd_booted checks for the same directory.
	if info, err := os.Stat("/run/systemd/system"); err != nil || !info.IsDir() {
		return errors.New("systemd is not running on this machine")
	}
	return nil
}
