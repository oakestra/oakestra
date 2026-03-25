package fsimg

import (
	"errors"
	"fmt"
	"go_node_engine/virtualization/internal/crosvm/internal/tailbuf"
	"os/exec"
	"strings"
)

func lookPathOrEmpty(file string) string {
	path, err := exec.LookPath(file)
	if err != nil {
		return ""
	}

	return path
}

func runCapturingOutput(stdoutCapacity int, stderrCapacity int, name string, arg ...string) error {
	var stdoutBuf *tailbuf.TailBuffer = nil
	if stdoutCapacity > 0 {
		stdoutBuf = tailbuf.NewTailBuffer(stdoutCapacity)
	}
	var stderrBuf *tailbuf.TailBuffer = nil
	if stderrCapacity > 0 {
		stderrBuf = tailbuf.NewTailBuffer(stderrCapacity)
	}

	cmd := exec.Command(name, arg...)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	if err := cmd.Run(); err != nil {
		return attachOutputToExitError(err, stdoutBuf, stderrBuf)
	}

	return nil
}

func attachOutputToExitError(err error, stdoutBuf *tailbuf.TailBuffer, stderrBuf *tailbuf.TailBuffer) error {
	if stdoutBuf == nil && stderrBuf == nil {
		return err
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return err
	}

	extendedErrBuilder := strings.Builder{}

	if stdoutBuf != nil {
		extendedErrBuilder.WriteString(fmt.Sprintf("\nstdout tail (max %d bytes):", stdoutBuf.Capacity()))

		stdoutBuilder := strings.Builder{}
		_, _ = stdoutBuf.WriteToSkippingUntil(&stdoutBuilder, tailbuf.IsValidUTF8Start)
		for line := range strings.Lines(stdoutBuilder.String()) {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 0 {
				extendedErrBuilder.WriteString("\n>   ")
				extendedErrBuilder.WriteString(trimmed)
			}
		}
	}

	if stderrBuf != nil {
		extendedErrBuilder.WriteString(fmt.Sprintf("\nstderr tail (max %d bytes):", stderrBuf.Capacity()))

		stderrBuilder := strings.Builder{}
		_, _ = stderrBuf.WriteToSkippingUntil(&stderrBuilder, tailbuf.IsValidUTF8Start)
		for line := range strings.Lines(stderrBuilder.String()) {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 0 {
				extendedErrBuilder.WriteString("\n>   ")
				extendedErrBuilder.WriteString(trimmed)
			}
		}
	}

	return fmt.Errorf("%w%s", exitErr, extendedErrBuilder.String())
}
