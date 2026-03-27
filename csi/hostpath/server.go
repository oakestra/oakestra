package main

import (
	csipb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
)

// newGRPCServer creates a gRPC server and registers the Identity and Node
// services implemented by this plugin.
func newGRPCServer(mountTracker *MountTracker) *grpc.Server {
	srv := grpc.NewServer()
	csipb.RegisterIdentityServer(srv, &identityServer{})
	csipb.RegisterNodeServer(srv, &nodeServer{
		nodeID:       *nodeID,
		mountTracker: mountTracker,
	})
	return srv
}
