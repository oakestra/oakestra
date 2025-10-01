package iotools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

func CopyFile(srcPath string, dstPath string, dstPerm os.FileMode) error {
	return CopyFileInFs(afero.NewOsFs(), srcPath, dstPath, dstPerm)
}

func CopyFileInFs(fs afero.Fs, srcPath string, dstPath string, dstPerm os.FileMode) error {
	srcFile, err := fs.OpenFile(srcPath, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer CloseOrWarn(srcFile, srcPath)

	dstFile, err := fs.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, dstPerm)
	if err != nil {
		return err
	}
	defer CloseOrWarn(dstFile, dstPath)

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return nil
}

func CopyDir(srcPath string, dstPath string) error {
	return CopyDirInFs(afero.NewOsFs(), srcPath, dstPath)
}

// CopyDirInFs recursively copies the directory at srcPath to dstPath, first deleting dstPath if it already exists
// Fails if srcPath is not a directory when the parent directory of dstPath doesn't exist yet.
//
// Links are not resolved when checking for overlap between srcPath and dstPath,
// which, if they do, can lead to errors or even infinite copy operations.
// Callers need to make sure srcPath and dstPath are not overlapping due to links.
func CopyDirInFs(fs afero.Fs, srcPath string, dstPath string) error {
	absSrcPath, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for source %q: %w", srcPath, err)
	}

	absDstPath, err := filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for destination %q: %w", dstPath, err)
	}

	// copying a directory to itself is a no-op
	if absSrcPath == absDstPath {
		return nil
	}

	// prevent copying a directory into itself (e.g., cp /a /a/b)
	if strings.HasPrefix(absDstPath, absSrcPath+string(os.PathSeparator)) {
		return fmt.Errorf("cannot copy directory %q into its child %q", srcPath, dstPath)
	}

	// prevent copying parent into a child that will be deleted (e.g., cp /a/b /a).
	if strings.HasPrefix(absSrcPath, absDstPath+string(os.PathSeparator)) {
		return fmt.Errorf("cannot copy directory %q into its parent %q", srcPath, dstPath)
	}

	srcInfo, err := fs.Stat(absSrcPath)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source %s is not a directory", srcPath)
	}

	// remove the destination directory if it exists
	if err := fs.RemoveAll(absDstPath); err != nil {
		return err
	}

	return copyDirInFsNoChecks(fs, absSrcPath, srcInfo.Mode(), absDstPath)
}

func copyDirInFsNoChecks(
	fs afero.Fs,
	srcPath string,
	srcMode os.FileMode,
	dstPath string,
) error {
	if err := fs.Mkdir(dstPath, srcMode); err != nil {
		return err
	}

	entries, err := afero.ReadDir(fs, srcPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcEntryPath := filepath.Join(srcPath, entry.Name())
		dstEntryPath := filepath.Join(dstPath, entry.Name())

		if entry.IsDir() {
			// recursively copy subdirectory
			if err := copyDirInFsNoChecks(fs, srcEntryPath, entry.Mode(), dstEntryPath); err != nil {
				return err
			}
		} else {
			if err := CopyFileInFs(fs, srcEntryPath, dstEntryPath, entry.Mode()); err != nil {
				return err
			}
		}
	}

	return nil
}
