package iotools_test

import (
	"go_node_engine/util/iotools"
	"os"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

// Helper to create a file with content.
func mustCreateFile(t *testing.T, fs afero.Fs, path, content string, perm os.FileMode) {
	err := afero.WriteFile(fs, path, []byte(content), perm)
	assert.NilError(t, err, "failed to create helper file %s", path)
}

// Helper to verify a file's content.
func assertFileContent(t *testing.T, fs afero.Fs, path, expectedContent string) {
	content, err := afero.ReadFile(fs, path)
	assert.NilError(t, err, "failed to read file for verification: %s", path)
	assert.Equal(t, expectedContent, string(content), "file content mismatch for %s", path)
}

// Helper to verify a file's permissions.
func assertFilePerm(t *testing.T, fs afero.Fs, path string, expectedPerm os.FileMode) {
	info, err := fs.Stat(path)
	assert.NilError(t, err, "failed to stat file for verification: %s", path)
	// We only check the permission bits, not the file type bits (like os.ModeDir).
	assert.Equal(t, expectedPerm.Perm(), info.Mode().Perm(), "file permission mismatch for %s", path)
}

// Helper to check if a path exists.
func assertPathExists(t *testing.T, fs afero.Fs, path string) {
	_, err := fs.Stat(path)
	assert.NilError(t, err, "expected path to exist: %s", path)
}

// Helper to check if a path does not exist.
func assertPathNotExists(t *testing.T, fs afero.Fs, path string) {
	_, err := fs.Stat(path)
	assert.Assert(t, os.IsNotExist(err), "expected path to not exist: %s", path)
}

func TestCopyDirInFs_BasicCopy(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Setup source directory
	assert.NilError(t, fs.MkdirAll("/src/subdir", 0755))
	mustCreateFile(t, fs, "/src/file1.txt", "content1", 0644)
	mustCreateFile(t, fs, "/src/subdir/file2.txt", "content2", 0600)

	// Setup destination parent directory
	assert.NilError(t, fs.Mkdir("/parent", 0755))

	// Perform the copy
	err := iotools.CopyDirInFs(fs, "/src", "/parent/dst")
	assert.NilError(t, err)

	// Verify the copy
	assertPathExists(t, fs, "/parent/dst")
	assertFileContent(t, fs, "/parent/dst/file1.txt", "content1")
	assertFilePerm(t, fs, "/parent/dst/file1.txt", 0644)
	assertFileContent(t, fs, "/parent/dst/subdir/file2.txt", "content2")
	assertFilePerm(t, fs, "/parent/dst/subdir/file2.txt", 0600)
}

func TestCopyDirInFs_OverwriteExistingDir(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Setup source directory
	assert.NilError(t, fs.Mkdir("/src", 0755))
	mustCreateFile(t, fs, "/src/new_file.txt", "new content", 0644)

	// Setup existing destination directory with old content
	assert.NilError(t, fs.Mkdir("/dst", 0755))
	mustCreateFile(t, fs, "/dst/old_file.txt", "old content", 0644)

	// Perform the copy
	err := iotools.CopyDirInFs(fs, "/src", "/dst")
	assert.NilError(t, err)

	// Verify old content is removed and new content is present
	assertPathExists(t, fs, "/dst/new_file.txt")
	assertPathNotExists(t, fs, "/dst/old_file.txt")
	assertFileContent(t, fs, "/dst/new_file.txt", "new content")
}

func TestCopyDirInFs_DstParentDoesNotExist(t *testing.T) {
	// afero.Fs.Mkdir is bugged in afero.NewMemMapFs: https://github.com/spf13/afero/issues/149
	fs := afero.NewBasePathFs(afero.NewOsFs(), t.TempDir())

	// Setup source
	assert.NilError(t, fs.Mkdir("/src", 0755))

	// Attempt to copy to a destination whose parent does not exist
	err := iotools.CopyDirInFs(fs, "/src", "/nonexistent_parent/dst")

	// Expect an error because the parent directory is required to exist
	assert.ErrorContains(t, err, "no such file or directory") // Specific error from fs.Mkdir
}

func TestCopyDirInFs_SourceDoesNotExist(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Setup destination parent
	assert.NilError(t, fs.Mkdir("/dst", 0755))

	// Attempt to copy from a non-existent source
	err := iotools.CopyDirInFs(fs, "/nonexistent_src", "/dst")

	// Expect an error
	assert.ErrorContains(t, err, "file does not exist")
}

func TestCopyDirInFs_SourceIsFile(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Setup source as a file
	mustCreateFile(t, fs, "/src_file.txt", "i am a file", 0644)

	// Setup destination parent
	assert.NilError(t, fs.Mkdir("/dst", 0755))

	// Attempt to copy a file as if it were a directory
	err := iotools.CopyDirInFs(fs, "/src_file.txt", "/dst")

	// Expect an error
	assert.ErrorContains(t, err, "is not a directory")
}

func TestCopyDirInFs_DstIsChildOfSrc(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Setup source directory
	assert.NilError(t, fs.Mkdir("/src", 0755))

	// Attempt to copy a directory into its own child
	err := iotools.CopyDirInFs(fs, "/src", "/src/dst")

	// Expect an error
	assert.ErrorContains(t, err, "cannot copy directory")
}

func TestCopyDirInFs_SrcIsChildOfDst(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Setup source as a child of destination
	assert.NilError(t, fs.MkdirAll("/dst/src", 0755))

	// Attempt to copy a child directory into its parent (which would be removed first)
	err := iotools.CopyDirInFs(fs, "/dst/src", "/dst")

	// Expect an error
	assert.ErrorContains(t, err, "cannot copy directory")
}

func TestCopyDirInFs_SrcAndDstAreSame(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Setup source directory
	assert.NilError(t, fs.Mkdir("/src", 0755))
	mustCreateFile(t, fs, "/src/file.txt", "original content", 0644)

	// Perform the copy (should be a no-op)
	err := iotools.CopyDirInFs(fs, "/src", "/src")
	assert.NilError(t, err)

	// Verify content remains unchanged
	assertFileContent(t, fs, "/src/file.txt", "original content")
}
