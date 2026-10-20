package kernel_test

import (
	"go_node_engine/virtualization/internal/crosvm/internal/kernel"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// newRootWithBoot creates a fake root filesystem whose /boot contains the given (empty) files.
func newRootWithBoot(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	boot := filepath.Join(root, "boot")
	assert.NilError(t, os.Mkdir(boot, 0o755))
	for _, f := range files {
		assert.NilError(t, os.WriteFile(filepath.Join(boot, f), nil, 0o644))
	}
	return root
}

func TestFindLinuxKernelFiles(t *testing.T) {
	cases := []struct {
		name       string
		files      []string
		wantKernel string
		wantInitrd string
	}{
		{
			// A plain string comparison would pick 5.9 here.
			name:       "versions compare numerically",
			files:      []string{"vmlinuz-5.9.0", "vmlinuz-5.10.0", "initrd.img-5.9.0", "initrd.img-5.10.0"},
			wantKernel: "vmlinuz-5.10.0",
			wantInitrd: "initrd.img-5.10.0",
		},
		{
			name:       "debian abi numbers",
			files:      []string{"vmlinuz-6.1.0-9-amd64", "vmlinuz-6.1.0-18-amd64", "initrd.img-6.1.0-18-amd64"},
			wantKernel: "vmlinuz-6.1.0-18-amd64",
			wantInitrd: "initrd.img-6.1.0-18-amd64",
		},
		{
			name:       "ubuntu flavours",
			files:      []string{"vmlinuz-6.8.0-45-generic", "vmlinuz-6.8.0-101-generic", "initrd.img-6.8.0-101-generic"},
			wantKernel: "vmlinuz-6.8.0-101-generic",
			wantInitrd: "initrd.img-6.8.0-101-generic",
		},
		{
			name:       "fedora style initramfs",
			files:      []string{"vmlinuz-6.11.4-301.fc41.x86_64", "initramfs-6.11.4-301.fc41.x86_64.img"},
			wantKernel: "vmlinuz-6.11.4-301.fc41.x86_64",
			wantInitrd: "initramfs-6.11.4-301.fc41.x86_64.img",
		},
		{
			// Compressed images are what bootloaders normally use, so they win over a raw vmlinux
			// even when the vmlinux looks newer.
			name:       "vmlinuz preferred over vmlinux",
			files:      []string{"vmlinuz-5.10.0", "vmlinux-6.0.0"},
			wantKernel: "vmlinuz-5.10.0",
		},
		{
			// Kernel-only images such as weaveworks/ignite-kernel ship just an uncompressed vmlinux.
			name:       "vmlinux fallback",
			files:      []string{"vmlinux", "vmlinux-5.10.77", "config-5.10.77"},
			wantKernel: "vmlinux-5.10.77",
		},
		{
			name:       "initrd is optional",
			files:      []string{"vmlinuz-6.1.0"},
			wantKernel: "vmlinuz-6.1.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newRootWithBoot(t, tc.files...)

			gotKernel, gotInitrd, err := kernel.FindLinuxKernelFiles(root)
			assert.NilError(t, err)

			assert.Equal(t, gotKernel, filepath.Join(root, "boot", tc.wantKernel))
			if tc.wantInitrd == "" {
				assert.Equal(t, gotInitrd, "")
			} else {
				assert.Equal(t, gotInitrd, filepath.Join(root, "boot", tc.wantInitrd))
			}
		})
	}
}

// Ordinary container images (alpine, debian, ...) have no kernel at all, which must be reported
// as ErrNoKernelFound so the crosvm runtime can tell the user the image is not a VM image.
func TestFindLinuxKernelFilesWithoutKernel(t *testing.T) {
	t.Run("empty boot directory", func(t *testing.T) {
		root := newRootWithBoot(t, "config-6.1.0", "System.map-6.1.0")
		_, _, err := kernel.FindLinuxKernelFiles(root)
		assert.ErrorIs(t, err, kernel.ErrNoKernelFound)
	})

	t.Run("no boot directory", func(t *testing.T) {
		_, _, err := kernel.FindLinuxKernelFiles(t.TempDir())
		assert.ErrorIs(t, err, kernel.ErrNoKernelFound)
	})
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		older, newer string
	}{
		{"5.9.0", "5.10.0"},
		{"6.1.0-9-amd64", "6.1.0-18-amd64"},
		{"6.8.0-45-generic", "6.8.0-101-generic"},
		{"4.19.215", "5.4.157"},
	}
	for _, tc := range cases {
		older, newer := kernel.NewVersion(tc.older), kernel.NewVersion(tc.newer)
		assert.Assert(t, newer.IsNewer(older), "%s should be newer than %s", tc.newer, tc.older)
		assert.Assert(t, !older.IsNewer(newer), "%s should not be newer than %s", tc.older, tc.newer)
	}
	assert.Equal(t, kernel.NewVersion("6.1.0").Compare(kernel.NewVersion("6.1.0")), 0)
}
