package cmd

import (
	"fmt"
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
		Run: func(cmd *cobra.Command, args []string) {
			logsNodeEngine()
		},
	}
)

func logsNodeEngine() error {
	// Open the log file
	logFile, err := os.Open("/var/log/oakestra/nodeengine.log")
	if err != nil {
		fmt.Println("Error opening log file, is the NodeEngine running? Use 'NodeEngine status' to check.")
		return err
	}
	defer logFile.Close()

	// Get the file size to start reading from the end
	fileInfo, err := logFile.Stat()
	if err != nil {
		return err
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
			return err
		}

		// Read new content from the file
		data, err := io.ReadAll(logFile)
		if err != nil && err != io.EOF {
			return err
		}

		fmt.Print(string(data))

		// Update the offset for the next read
		offset += int64(len(data))
	}
}
