package csi

import (
	"context"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"os"
	"path/filepath"
	"time"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	// oakStagingBase is the node-global staging directory root.
	oakStagingBase = "/var/lib/oakestra/csi/staging"

	// oakPublishBase is the per-instance publish directory root.
	oakPublishBase = "/var/lib/oakestra/csi/publish"
)

// MountedVolume records the paths produced by a successful CSI mount sequence.
type MountedVolume struct {
	VolumeID    string
	DriverName  string
	StagingPath string // empty when StageUnstageRequired is false
	TargetPath  string // host-side published path; bind-mount source
	MountPath   string // destination path inside the container
}

// MountVolumes performs the CSI Stage + Publish sequence for every VolumeRequest
// attached to a service. On partial failure the already-mounted volumes are
// returned alongside the error so callers can pass them to UnmountVolumes.
func MountVolumes(service model.Service) ([]MountedVolume, error) {
	mounted := make([]MountedVolume, 0, len(service.Volumes))
	for _, vol := range service.Volumes {
		mv, err := mountSingle(service, vol)
		if err != nil {
			return mounted, fmt.Errorf("volume %s (%s): %w", vol.VolumeID, vol.CSIDriver, err)
		}
		mounted = append(mounted, mv)
	}
	return mounted, nil
}

// UnmountVolumes reverses the CSI Publish + Unstage sequence for the given
// mounted volumes. Safe to call even if volumes were only partially mounted.
func UnmountVolumes(mounted []MountedVolume) {
	for _, mv := range mounted {
		if err := unmountSingle(mv); err != nil {
			logger.ErrorLogger().Printf("[CSI] Unmount error for volume %s: %v", mv.VolumeID, err)
		}
	}
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func mountSingle(service model.Service, vol model.VolumeRequest) (MountedVolume, error) {
	reg := GetRegistry()
	plugin, err := reg.Get(vol.CSIDriver)
	if err != nil {
		return MountedVolume{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mv := MountedVolume{
		VolumeID:   vol.VolumeID,
		DriverName: plugin.DriverName,
		MountPath:  vol.MountPath,
	}

	// Stage (if the plugin requires it)
	if plugin.StageUnstageRequired {
		stagingPath := stagingDir(plugin.DriverName, vol.VolumeID)
		if err := os.MkdirAll(stagingPath, 0750); err != nil {
			return MountedVolume{}, fmt.Errorf("mkdir staging %s: %w", stagingPath, err)
		}
		_, err = plugin.nodeRPCClient().NodeStageVolume(ctx, &csipb.NodeStageVolumeRequest{
			VolumeId:          vol.VolumeID,
			StagingTargetPath: stagingPath,
			VolumeCapability:  defaultVolumeCapability(),
			VolumeContext:     vol.Config,
		})
		if err != nil {
			return MountedVolume{}, fmt.Errorf("NodeStageVolume: %w", err)
		}
		mv.StagingPath = stagingPath
		logger.InfoLogger().Printf("[CSI] Staged volume %s at %s", vol.VolumeID, stagingPath)
	}

	// Publish
	targetPath := publishDir(plugin.DriverName, service.Sname, service.Instance, vol.VolumeID)
	if err := os.MkdirAll(targetPath, 0750); err != nil {
		return mv, fmt.Errorf("mkdir publish %s: %w", targetPath, err)
	}
	_, err = plugin.nodeRPCClient().NodePublishVolume(ctx, &csipb.NodePublishVolumeRequest{
		VolumeId:          vol.VolumeID,
		StagingTargetPath: mv.StagingPath,
		TargetPath:        targetPath,
		VolumeCapability:  defaultVolumeCapability(),
		Readonly:          false,
		VolumeContext:     vol.Config,
	})
	if err != nil {
		return mv, fmt.Errorf("NodePublishVolume: %w", err)
	}
	mv.TargetPath = targetPath
	logger.InfoLogger().Printf("[CSI] Published volume %s at %s", vol.VolumeID, targetPath)

	return mv, nil
}

func unmountSingle(mv MountedVolume) error {
	reg := GetRegistry()
	plugin, err := reg.Get(mv.DriverName)
	if err != nil {
		logger.ErrorLogger().Printf("[CSI] Plugin %s not found during unmount of %s: %v",
			mv.DriverName, mv.VolumeID, err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Unpublish
	if mv.TargetPath != "" {
		_, err = plugin.nodeRPCClient().NodeUnpublishVolume(ctx, &csipb.NodeUnpublishVolumeRequest{
			VolumeId:   mv.VolumeID,
			TargetPath: mv.TargetPath,
		})
		if err != nil {
			logger.ErrorLogger().Printf("[CSI] NodeUnpublishVolume %s: %v", mv.VolumeID, err)
		} else {
			logger.InfoLogger().Printf("[CSI] Unpublished volume %s from %s", mv.VolumeID, mv.TargetPath)
		}
		_ = os.Remove(mv.TargetPath)
	}

	// Unstage
	if mv.StagingPath != "" {
		_, err = plugin.nodeRPCClient().NodeUnstageVolume(ctx, &csipb.NodeUnstageVolumeRequest{
			VolumeId:          mv.VolumeID,
			StagingTargetPath: mv.StagingPath,
		})
		if err != nil {
			logger.ErrorLogger().Printf("[CSI] NodeUnstageVolume %s: %v", mv.VolumeID, err)
		} else {
			logger.InfoLogger().Printf("[CSI] Unstaged volume %s from %s", mv.VolumeID, mv.StagingPath)
		}
		_ = os.Remove(mv.StagingPath)
	}

	return nil
}

// defaultVolumeCapability returns a SINGLE_NODE_WRITER mount-mode capability.
func defaultVolumeCapability() *csipb.VolumeCapability {
	return &csipb.VolumeCapability{
		AccessType: &csipb.VolumeCapability_Mount{
			Mount: &csipb.VolumeCapability_MountVolume{},
		},
		AccessMode: &csipb.VolumeCapability_AccessMode{
			Mode: csipb.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
}

func stagingDir(driverName, volumeID string) string {
	return filepath.Join(oakStagingBase, driverName, volumeID)
}

func publishDir(driverName, sname string, instance int, volumeID string) string {
	return filepath.Join(oakPublishBase, driverName, sname, fmt.Sprintf("%d", instance), volumeID)
}
