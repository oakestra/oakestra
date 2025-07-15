package utils

import (
	"io"
	"os"
)

// OnceReader is an interface that wraps the Read method.
// It is used to read data from a source only once.
// After the read is finished, it closes the resource and deletes the file.
type OnceReader interface {
	Read(p []byte) (n int, err error)
	GetFile() *os.File
}

type deletingReadCloser struct {
	io.ReadCloser
	file *os.File
}

func NewOnceReader(file *os.File) OnceReader {
	return &deletingReadCloser{
		ReadCloser: io.NopCloser(file), // Open the file for reading
		file:       file,
	}
}

func (drc *deletingReadCloser) GetFile() *os.File {
	return drc.file
}

func (drc *deletingReadCloser) Read(p []byte) (n int, err error) {
	if drc.ReadCloser == nil {
		return 0, io.EOF // If the ReadCloser is nil, return EOF
	}

	n, err = drc.ReadCloser.Read(p)
	if err != nil {
		drc.cleanup()
		return n, err
	}

	return n, nil
}

// Close closes the underlying reader and then deletes the file.
func (drc *deletingReadCloser) cleanup() error {

	if drc.file != nil {
		if err := os.Remove(drc.file.Name()); err != nil {
			return err
		}
	}

	if drc.ReadCloser != nil {
		if err := drc.ReadCloser.Close(); err != nil {
			return err
		}
	}

	return nil
}
