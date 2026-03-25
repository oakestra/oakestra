package iotools

import (
	"go_node_engine/logger"
	"io"
)

func CloseOrWarn(closer io.Closer, name string) {
	if err := closer.Close(); err != nil {
		logger.WarnLogger().Printf("failed to close %q: %v", name, err)
	}
}
