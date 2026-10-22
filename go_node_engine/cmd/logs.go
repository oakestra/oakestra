package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	nodeEngineLogPath = "/var/log/oakestra/nodeengine.log"
	// logPollInterval is how often a followed log file is checked for new lines.
	logPollInterval = 500 * time.Millisecond
	// maxTailBytes bounds how much of the file is read to find the last lines.
	maxTailBytes = 1 << 20
)

var logsTailLines int

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().IntVarP(&logsTailLines, "tail", "n", 50, "number of existing lines to show before following")
}

var (
	logsCmd = &cobra.Command{
		Use:   "logs",
		Short: "tail check node engine logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return logsNodeEngine()
		},
	}
)

func logsNodeEngine() error {
	return followLog(nodeEngineLogPath, logsTailLines, func() { fmt.Println("\nDetatching...") })
}

// followLog prints the last tail lines of path, then follows new lines until
// interrupted, calling onInterrupt first. It reopens the file when logrotate
// replaces or truncates it.
func followLog(path string, tail int, onInterrupt func()) error {
	logFile, err := os.Open(path)
	if err != nil {
		fmt.Println("Error opening log file, is the NodeEngine running? Use 'NodeEngine status' to check.")
		return err
	}
	defer func() { _ = logFile.Close() }()

	offset, err := printLastLines(os.Stdout, logFile, tail)
	if err != nil {
		return err
	}

	// Handle interrupt signal (Ctrl+C)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		onInterrupt()
		os.Exit(0)
	}()

	for {
		if pathInfo, err := os.Stat(path); err == nil {
			fileInfo, err := logFile.Stat()
			if err == nil && !os.SameFile(pathInfo, fileInfo) {
				// Rotated: drain the old file, then continue with the new one.
				if _, err := logFile.Seek(offset, io.SeekStart); err == nil {
					_, _ = io.Copy(os.Stdout, logFile)
				}
				if reopened, err := os.Open(path); err == nil {
					_ = logFile.Close()
					logFile, offset = reopened, 0
				}
			} else if err == nil && fileInfo.Size() < offset {
				offset = 0 // truncated in place
			}
		}

		if _, err := logFile.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		n, err := io.Copy(os.Stdout, logFile)
		if err != nil {
			return err
		}
		offset += n
		time.Sleep(logPollInterval)
	}
}

// printLastLines writes up to n trailing lines of f to w and returns the end offset.
func printLastLines(w io.Writer, f *os.File, n int) (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	if n <= 0 || size == 0 {
		return size, nil
	}
	start := max(size-maxTailBytes, 0)
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return 0, err
	}
	// Skip the trailing newline, then walk back n line breaks.
	cut := len(buf)
	if cut > 0 && buf[cut-1] == '\n' {
		cut--
	}
	for i := 0; i < n; i++ {
		idx := bytes.LastIndexByte(buf[:cut], '\n')
		if idx < 0 {
			cut = -1
			break
		}
		cut = idx
	}
	_, err = w.Write(buf[cut+1:])
	return size, err
}
