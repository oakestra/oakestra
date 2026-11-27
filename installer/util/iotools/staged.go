package iotools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/spf13/afero"
)

type stagedFile struct {
	tempPath string
	dstPath  string
}

// StagedFiles holds files prepared in their destination directories.
// Close removes any files that have not been installed.
type StagedFiles struct {
	fs    afero.Fs
	files []stagedFile
}

func NewStagedFiles() *StagedFiles {
	return NewStagedFilesInFs(afero.NewOsFs())
}

func NewStagedFilesInFs(fs afero.Fs) *StagedFiles {
	return &StagedFiles{fs: fs}
}

// Prepare writes and syncs a temporary file in the destination directory.
func (s *StagedFiles) Prepare(ctx context.Context, dstPath string, perm os.FileMode, reader io.Reader) error {
	tempPath, err := prepareFileInFs(ctx, s.fs, dstPath, perm, reader)
	if err != nil {
		return fmt.Errorf("prepare %q: %w", dstPath, err)
	}
	file := stagedFile{tempPath: tempPath, dstPath: dstPath}
	index := slices.IndexFunc(s.files, func(file stagedFile) bool { return file.dstPath == dstPath })
	if index < 0 {
		s.files = append(s.files, file)
		return nil
	}
	if err := s.fs.Remove(s.files[index].tempPath); err != nil {
		return errors.Join(err, s.fs.Remove(tempPath))
	}
	s.files[index] = file
	return nil
}

// Commit checks for cancellation before starting the renames.
// If a rename fails, earlier renames remain in effect.
func (s *StagedFiles) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for len(s.files) > 0 {
		file := s.files[0]
		if err := s.fs.Rename(file.tempPath, file.dstPath); err != nil {
			return fmt.Errorf("install %q: %w", file.dstPath, err)
		}
		s.files = s.files[1:]
	}
	return nil
}

func (s *StagedFiles) Close() error {
	var cleanupErr error
	for _, file := range s.files {
		if err := s.fs.Remove(file.tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove temporary file %q: %w", file.tempPath, err))
		}
	}
	s.files = nil
	return cleanupErr
}
