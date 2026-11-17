package kernel

import (
	"fmt"
	"path/filepath"
	"slices"
)

var ErrNoKernelFound = fmt.Errorf("no kernel found")

var (
	kernelPatterns = []string{"vmlinuz-*", "vmlinux-*"}
	initrdPatterns = []string{"initramfs-*", "initrd-*", "initrd.img-*"}
)

// FindLinuxKernelFiles tries to find kernel and initrd images under root and returns (kernelPath, initrdPath, err).
// If no kernel is found ErrNoKernelFound is returned,
// while not finding an initrd image will only result in an empty string for its path.
func FindLinuxKernelFiles(root string) (string, string, error) {
	boot := filepath.Join(root, "boot")

	kernels, err := findFirstMatches(boot, kernelPatterns)
	if err != nil {
		return "", "", err
	}

	if len(kernels) == 0 {
		return "", "", ErrNoKernelFound
	}
	latestKernel := slices.MaxFunc(kernels, compareKernelPaths)

	initrds, err := findFirstMatches(boot, initrdPatterns)
	if err != nil {
		return "", "", err
	}

	latestInitrd := ""
	if len(initrds) > 0 {
		latestInitrd = slices.MaxFunc(initrds, compareKernelPaths)
	}

	return latestKernel, latestInitrd, nil
}

func compareKernelPaths(a string, b string) int {
	va := NewVersion(filepath.Base(a))
	vb := NewVersion(filepath.Base(b))
	return va.Compare(vb)
}

// findFirstMatches tries each pattern in order, returning on the first non-empty glob result.
func findFirstMatches(dir string, patterns []string) ([]string, error) {
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			return matches, nil
		}
	}
	return nil, nil
}
