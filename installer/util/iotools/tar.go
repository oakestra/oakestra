package iotools

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/afero"
)

type TarTarget interface {
	match(name string) (dstPath string, perm os.FileMode)
	pattern() string
}

type ExactTarTarget struct {
	TarName string
	DstPath string
	DstPerm os.FileMode
}

func (e ExactTarTarget) match(name string) (string, os.FileMode) {
	if e.TarName != name {
		return "", 0
	}
	return e.DstPath, e.DstPerm
}

func (e ExactTarTarget) pattern() string {
	return e.TarName
}

type RegexpTarTarget struct {
	TarNameRegex    *regexp.Regexp
	DstPathTemplate string
	DstPerm         os.FileMode
}

func (r RegexpTarTarget) match(name string) (string, os.FileMode) {
	if !r.TarNameRegex.MatchString(name) {
		return "", 0
	}
	return r.TarNameRegex.ReplaceAllString(name, r.DstPathTemplate), r.DstPerm
}

func (r RegexpTarTarget) pattern() string {
	return r.TarNameRegex.String()
}

// ExtractTar extracts regular files from an uncompressed TAR archive.
func ExtractTar(ctx context.Context, archivePath string, targets ...TarTarget) error {
	return ExtractTarInFs(ctx, afero.NewOsFs(), archivePath, targets...)
}

func ExtractTarInFs(ctx context.Context, fs afero.Fs, archivePath string, targets ...TarTarget) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := fs.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	staged := NewStagedFilesInFs(fs)
	defer func() { err = errors.Join(err, staged.Close()) }()

	if err := staged.readTar(ctx, file, targets); err != nil {
		return fmt.Errorf("extract archive %q: %w", archivePath, err)
	}
	return staged.Commit(ctx)
}

// ExtractTarGzip extracts regular files from a Gzip-compressed TAR archive.
func ExtractTarGzip(ctx context.Context, archivePath string, targets ...TarTarget) error {
	return ExtractTarGzipInFs(ctx, afero.NewOsFs(), archivePath, targets...)
}

func ExtractTarGzipInFs(ctx context.Context, fs afero.Fs, archivePath string, targets ...TarTarget) (err error) {
	staged := NewStagedFilesInFs(fs)
	defer func() { err = errors.Join(err, staged.Close()) }()

	if err := staged.StageTarGzip(ctx, archivePath, targets...); err != nil {
		return err
	}
	return staged.Commit(ctx)
}

// StageTarGzip prepares files from one archive and verifies its checksum.
// On error, the batch may contain unverified files. The caller owns cleanup.
func (s *StagedFiles) StageTarGzip(ctx context.Context, archivePath string, targets ...TarTarget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := s.fs.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(contextReader{ctx: ctx, reader: file})
	if err != nil {
		return fmt.Errorf("read archive %q: %w", archivePath, err)
	}
	defer func() { _ = gzipReader.Close() }()

	if err := s.readTar(ctx, gzipReader, targets); err != nil {
		return fmt.Errorf("extract archive %q: %w", archivePath, err)
	}
	// The gzip reader must be read to EOF to verify the gzip checksum.
	if _, err := io.Copy(io.Discard, contextReader{ctx: ctx, reader: gzipReader}); err != nil {
		return fmt.Errorf("verify archive %q: %w", archivePath, err)
	}
	return ctx.Err()
}

func (s *StagedFiles) readTar(ctx context.Context, reader io.Reader, targets []TarTarget) error {
	tarReader := tar.NewReader(contextReader{ctx: ctx, reader: reader})
	matched := make([]bool, len(targets))
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		name := filepath.Clean(header.Name)
		for index, target := range targets {
			dstPath, perm := target.match(name)
			if dstPath == "" {
				continue
			}
			if err := s.Prepare(ctx, dstPath, perm, tarReader); err != nil {
				return err
			}
			matched[index] = true
			break
		}
	}
	for index, target := range targets {
		if !matched[index] {
			return fmt.Errorf("missing regular file matching %q", target.pattern())
		}
	}
	return nil
}
