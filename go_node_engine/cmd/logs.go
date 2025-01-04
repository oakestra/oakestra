package cmd

import (
	"fmt"
	"go_node_engine/logger"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(logsCmd)
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
	logFile, err := os.Open("/var/log/oakestra/nodeengine.log")
	if err != nil {
		return fmt.Errorf("error opening log file, is the NodeEngine running? Use 'NodeEngine status' to check: %v", err)
	}
	defer func() {
		err := logFile.Close()
		if err != nil {
			logger.ErrorLogger().Printf("Unable to close logfile")
		}
	}()

	// Get the file size to start reading from the end
	fileInfo, err := logFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get log file info: %v", err)
	}

	// Track the current position in the file
	offset := fileInfo.Size()

	// Handle interrupt signal (Ctrl+C)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nDetatching...")
		os.Exit(0)
	}()

	// Continuously tail the log file
	for {
		// Seek to the end of the file
		_, err = logFile.Seek(offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek in log file: %v", err)
		}

		// Read new content from the file
		data, err := io.ReadAll(logFile)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read log file content: %v", err)
		}

		fmt.Print(string(data))

		// Update the offset for the next read
		offset += int64(len(data))
	}
}
