package main

import (
	"context"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	driverName    = "csi.oakestra.io/hostpath"
	driverVersion = "0.1.0"
)

// identityServer implements the CSI Identity service.
// It advertises no plugin-level capabilities because this is a headless
// Node-only driver (no controller).
type identityServer struct {
	csipb.UnimplementedIdentityServer
}

// GetPluginInfo returns the driver name and version.
func (i *identityServer) GetPluginInfo(
	_ context.Context,
	_ *csipb.GetPluginInfoRequest,
) (*csipb.GetPluginInfoResponse, error) {
	return &csipb.GetPluginInfoResponse{
		Name:          driverName,
		VendorVersion: driverVersion,
	}, nil
}

// GetPluginCapabilities signals that this driver has no controller service
// and does not implement volume accessibility constraints.
func (i *identityServer) GetPluginCapabilities(
	_ context.Context,
	_ *csipb.GetPluginCapabilitiesRequest,
) (*csipb.GetPluginCapabilitiesResponse, error) {
	// Empty capability list: no CONTROLLER_SERVICE, no VOLUME_ACCESSIBILITY_CONSTRAINTS.
	return &csipb.GetPluginCapabilitiesResponse{
		Capabilities: []*csipb.PluginCapability{},
	}, nil
}

// Probe reports that the plugin is healthy and ready to serve requests.
func (i *identityServer) Probe(
	_ context.Context,
	_ *csipb.ProbeRequest,
) (*csipb.ProbeResponse, error) {
	return &csipb.ProbeResponse{}, nil
}
