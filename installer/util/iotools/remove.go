package iotools

import (
	"log/slog"

	"github.com/spf13/afero"
)

func RemoveAllOrWarn(logger *slog.Logger, targetPath string) {
	RemoveAllOrWarnInFs(afero.NewOsFs(), logger, targetPath)
}

func RemoveAllOrWarnInFs(fs afero.Fs, logger *slog.Logger, targetPath string) {
	if err := fs.RemoveAll(targetPath); err != nil {
		logger.Warn("Failed to remove temporary files", "path", targetPath, "err", err)
	}
}
