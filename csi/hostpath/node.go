package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// nodeServer implements the CSI Node service.
//
// Design
// ──────
// This is a headless plugin: only NodePublishVolume / NodeUnpublishVolume are
// meaningfully implemented. STAGE_UNSTAGE_VOLUME is NOT advertised, so the
// Oakestra Node Engine will skip the staging phase entirely.
//
// Volume mounting flow
// ────────────────────
//
//	Deployment descriptor (user)
//	  volumes:
//	    - volume_id:   /host/data          ← host-side source directory (A)
//	      csi_driver:  csi.oakestra.io/hostpath
//	      mount_path:  /container/data     ← destination inside the container (B)
//
//	NodePublishVolume called by Oakestra Node Engine:
//	  VolumeId   = "/host/data"            (or from VolumeContext["host_path"])
//	  TargetPath = /var/lib/oakestra/csi/publish/…  (host-side staging point)
//
//	This plugin:  bind-mount  /host/data  →  TargetPath
//
//	Oakestra Node Engine then adds to the OCI container spec:
//	              bind-mount  TargetPath   →  /container/data
type nodeServer struct {
	csipb.UnimplementedNodeServer

	nodeID       string
	mountTracker *MountTracker
}

// NodePublishVolume bind-mounts the host directory onto the CSI target path.
//
// Source resolution (in priority order):
//  1. VolumeContext["host_path"] – explicit override in the deployment descriptor's
//     `config` map.
//  2. VolumeId – treated directly as an absolute host-side path.
//
// The bind-mount is recursive (MS_BIND|MS_REC) so that any sub-mounts inside
// the source directory remain visible inside the container.
func (n *nodeServer) NodePublishVolume(
	_ context.Context,
	req *csipb.NodePublishVolumeRequest,
) (*csipb.NodePublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumeId is required")
	}
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "TargetPath is required")
	}

	// Resolve source directory (directory A on the host).
	sourcePath := volumeID
	if hp, ok := req.GetVolumeContext()["host_path"]; ok && hp != "" {
		sourcePath = hp
	}

	// BASE_MOUNT_PATH is an optional environment variable that, if set, is prefixed to all source paths.
	if base := os.Getenv("BASE_MOUNT_PATH"); base != "" {
		// remove any ../ or ./ from the sourcePath to prevent escaping the base directory
		sourcePath = strings.ReplaceAll(sourcePath, "../", "")
		sourcePath = strings.ReplaceAll(sourcePath, "./", "")
		sourcePath = fmt.Sprintf("%s/%s", base, sourcePath)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		// stat failed, create path if not exist
		if os.IsNotExist(err) {
			if err := os.MkdirAll(sourcePath, 0750); err != nil {
				return nil, status.Errorf(codes.Internal,
					"mkdir source %q: %v", sourcePath, err)
			}
			log.Printf("[hostpath] Created source directory %q", sourcePath)
		} else {
			// stat failed for another reason
			return nil, status.Errorf(codes.NotFound,
				"host path %q not found: %v", sourcePath, err)
		}
	}

	// Ensure the target directory exists (Node Engine already creates it, but
	// be defensive in case this is called standalone).
	if err := os.MkdirAll(targetPath, 0750); err != nil {
		return nil, status.Errorf(codes.Internal,
			"mkdir target %q: %v", targetPath, err)
	}

	// Check if already mounted to prevent duplicate mounts
	// First check our tracker (fast)
	if n.mountTracker != nil && n.mountTracker.IsMounted(targetPath) {
		log.Printf("[hostpath] Volume %q already tracked as mounted at %s, skipping duplicate mount", volumeID, targetPath)
		return &csipb.NodePublishVolumeResponse{}, nil
	}

	// Also check the kernel mount table (authoritative source of truth)
	if alreadyMounted, err := isMountPoint(targetPath); err != nil {
		log.Printf("[hostpath] Warning: failed to check mount status for %s: %v", targetPath, err)
	} else if alreadyMounted {
		log.Printf("[hostpath] Volume %q already mounted at %s (detected in /proc/mounts), skipping duplicate mount", volumeID, targetPath)
		// This path was mounted but not tracked - add it to tracking for proper cleanup
		if n.mountTracker != nil {
			_ = n.mountTracker.AddMount(volumeID, sourcePath, targetPath)
		}
		return &csipb.NodePublishVolumeResponse{}, nil
	}

	// Perform the recursive bind-mount: A → targetPath.
	// The Node Engine will subsequently bind-mount targetPath → mount_path (B)
	// inside the container OCI spec.
	flags := uintptr(syscall.MS_BIND | syscall.MS_REC)
	if req.GetReadonly() {
		flags |= syscall.MS_RDONLY
	}
	if err := syscall.Mount(sourcePath, targetPath, "", flags, ""); err != nil {
		return nil, status.Errorf(codes.Internal,
			"bind-mount %q → %q: %v", sourcePath, targetPath, err)
	}

	// Track the mount for cleanup resilience
	if n.mountTracker != nil {
		if err := n.mountTracker.AddMount(volumeID, sourcePath, targetPath); err != nil {
			log.Printf("[hostpath] Warning: failed to track mount: %v", err)
			// Don't fail the operation - the mount succeeded
		}
	}

	log.Printf("[hostpath] Published volume %q:  %s  →  %s", volumeID, sourcePath, targetPath)
	return &csipb.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the bind-mount at TargetPath and removes the
// now-empty directory. If unmount fails due to resource being busy, it will
// be retried in the background.
func (n *nodeServer) NodeUnpublishVolume(
	_ context.Context,
	req *csipb.NodeUnpublishVolumeRequest,
) (*csipb.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "TargetPath is required")
	}

	// Use mount tracker for resilient cleanup
	if n.mountTracker != nil {
		if err := n.mountTracker.RemoveMount(targetPath); err != nil {
			// Don't fail the RPC - cleanup will be retried in background
			log.Printf("[hostpath] Unmount queued for retry: %s", targetPath)
		} else {
			log.Printf("[hostpath] Unpublished volume from %s", targetPath)
		}
	} else {
		// Fallback to direct unmount if no tracker (shouldn't happen)
		if err := syscall.Unmount(targetPath, 0); err != nil {
			if !os.IsNotExist(err) {
				log.Printf("[hostpath] Warning: unmount %q: %v", targetPath, err)
			}
		} else {
			log.Printf("[hostpath] Unpublished volume from %s", targetPath)
		}

		if err := os.RemoveAll(targetPath); err != nil {
			log.Printf("[hostpath] Warning: remove %q: %v", targetPath, err)
		}
	}

	return &csipb.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities reports the set of node-level capabilities.
// This plugin does NOT advertise STAGE_UNSTAGE_VOLUME, meaning the Oakestra
// Node Engine will skip NodeStageVolume / NodeUnstageVolume entirely.
func (n *nodeServer) NodeGetCapabilities(
	_ context.Context,
	_ *csipb.NodeGetCapabilitiesRequest,
) (*csipb.NodeGetCapabilitiesResponse, error) {
	return &csipb.NodeGetCapabilitiesResponse{
		Capabilities: []*csipb.NodeServiceCapability{},
	}, nil
}

// NodeGetInfo returns the node identifier used for topology decisions.
func (n *nodeServer) NodeGetInfo(
	_ context.Context,
	_ *csipb.NodeGetInfoRequest,
) (*csipb.NodeGetInfoResponse, error) {
	resp := &csipb.NodeGetInfoResponse{
		MaxVolumesPerNode: 0, // unlimited
	}
	if n.nodeID != "" {
		resp.NodeId = n.nodeID
	} else {
		// Fall back to the OS hostname when no explicit node ID was provided.
		hostname, err := os.Hostname()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "hostname: %v", err)
		}
		resp.NodeId = hostname
	}
	return resp, nil
}

// NodeGetVolumeStats returns usage statistics for a published volume path.
// This is optional; Oakestra does not currently call it.
func (n *nodeServer) NodeGetVolumeStats(
	_ context.Context,
	req *csipb.NodeGetVolumeStatsRequest,
) (*csipb.NodeGetVolumeStatsResponse, error) {
	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumePath is required")
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(volumePath, &stat); err != nil {
		return nil, status.Errorf(codes.Internal, "statfs %q: %v", volumePath, err)
	}

	total := int64(stat.Blocks) * stat.Bsize     //nolint:unconvert
	avail := int64(stat.Bavail) * stat.Bsize     //nolint:unconvert
	used := total - int64(stat.Bfree)*stat.Bsize //nolint:unconvert

	return &csipb.NodeGetVolumeStatsResponse{
		Usage: []*csipb.VolumeUsage{
			{
				Unit:      csipb.VolumeUsage_BYTES,
				Total:     total,
				Available: avail,
				Used:      used,
			},
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Stubs for operations that are not applicable to a hostpath driver
// ---------------------------------------------------------------------------

// NodeStageVolume is never called because STAGE_UNSTAGE_VOLUME is not
// advertised in NodeGetCapabilities.
func (n *nodeServer) NodeStageVolume(
	_ context.Context,
	_ *csipb.NodeStageVolumeRequest,
) (*csipb.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented,
		fmt.Sprintf("%s does not support NodeStageVolume", driverName))
}

// NodeUnstageVolume – see NodeStageVolume.
func (n *nodeServer) NodeUnstageVolume(
	_ context.Context,
	_ *csipb.NodeUnstageVolumeRequest,
) (*csipb.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented,
		fmt.Sprintf("%s does not support NodeUnstageVolume", driverName))
}

// NodeExpandVolume – online expansion of hostpath volumes is not supported.
func (n *nodeServer) NodeExpandVolume(
	_ context.Context,
	_ *csipb.NodeExpandVolumeRequest,
) (*csipb.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented,
		fmt.Sprintf("%s does not support NodeExpandVolume", driverName))
}
