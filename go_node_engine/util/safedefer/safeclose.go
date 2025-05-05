package safedefer

import (
	"go_node_engine/logger"
	"io"
)

func SafeClose(closer io.Closer, name string) {
	if err := closer.Close(); err != nil {
		logger.WarnLogger().Printf("safedefer: failed to close %q: %v", name, err)
	}
}

func SafeDefer(closeFunc func() error, message string) {
	if err := closeFunc(); err != nil {
		logger.WarnLogger().Printf("safedefer: %s: %v", message, err)
	}
}
