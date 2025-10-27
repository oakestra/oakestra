package utils

import (
	rt "runtime"

	"github.com/vishvananda/netns"
)

// Execute function inside a namespace based on Ns name
func ExecInsideNsByName(Nsname string, function func() error) error {
	var containerNs netns.NsHandle

	rt.LockOSThread()
	defer rt.UnlockOSThread()

	stdNetns, err := netns.Get()
	if err == nil {
		defer func() {
			_ = stdNetns.Close()
		}()
		containerNs, err = netns.GetFromName(Nsname)
		if err == nil {
			defer func() {
				_ = netns.Set(stdNetns)
			}()
			err = netns.Set(containerNs)
			if err == nil {
				err = function()
			}
		}
	}
	return err
}

func CreateNetnsByName(Nsname string) error {
	newNs, err := netns.NewNamed(Nsname)
	if err != nil {
		return err
	}
	return newNs.Close()
}
