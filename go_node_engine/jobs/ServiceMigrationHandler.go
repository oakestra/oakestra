package jobs

import (
	"context"
	"fmt"
	"go_node_engine/model"
	pb "go_node_engine/requests/proto" // Adjust import path if needed
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
)

var migrationServiceSingletonOnce sync.Once
var migrationServiceSingleton *serviceMigrationHandler

type MigrationDetails struct {
	SystemJobID     string `json:"job_id"`
	JobName         string `json:"job_name"`
	Virtualization  string `json:"virtualization"`
	InstanceNumber  int    `json:"instance_number"`
	TargetNodeID    string `json:"target_node_id"`
	TargetNodeIP    string `json:"target_node_ip"`
	TargetNodePort  string `json:"target_node_port"`
	MigrationToken  string `json:"migration_token"`
	MigrationScheme string `json:"migration_scheme"`
	// Additional fields can be added as needed
	lastUpdated     time.Time // Timestamp of the last update to the migration details
	acknowledged    bool      // Indicates if the migration has been acknowledged
	dataTransferred bool      // Indicates if the data has been transferred
}

// ServiceMigrationHandler implements the MigrationServiceServer interface.
type serviceMigrationHandler struct {
	pb.UnimplementedMigrationServiceServer
	migrations    map[string]*MigrationDetails // Maps service ID to current migration
	migrations_mu sync.Mutex
	server        *grpc.Server // gRPC server instance
}

// ServiceMigrationHandler is responsible for handling service migration oeprations.
type MigrationHandler interface {
	AddIncomingMigration(MigrationDetails) error // Adds details for an incoming migration from another node.
	Migrate(MigrationDetails) error              // Performs a migration to another node.
}

// Get a singleton instance of the migration handler.
func GetMigrationHandler() MigrationHandler {
	migrationServiceSingletonOnce.Do(func() {
		migrationServiceSingleton = &serviceMigrationHandler{
			migrations: make(map[string]*MigrationDetails),
		}
		go func() {
			err := migrationServiceSingleton.startMigrationServer(
				fmt.Sprintf("[::]:%s", model.GetNodeInfo().Port), // Replace with your desired address and port
			)
			if err != nil {
				log.Fatalf("Failed to start migration server: %v", err)
			}
		}()
	})
	return migrationServiceSingleton
}

// StopMigrationHandler stops the migration server gracefully.
func StopMigrationHandler() {
	migrationServiceSingleton.server.GracefulStop()
	log.Println("Migration server stopped gracefully")
}

// StartMigrationServer starts the gRPC server to receive migrations.
func (h *serviceMigrationHandler) startMigrationServer(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	h.server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	pb.RegisterMigrationServiceServer(h.server, h)
	log.Printf("Migration server listening at %s", address)
	return h.server.Serve(lis)
}

// AddIncomingMigration adds a new incoming migration to the migration handler.
// A migration client has 30 seconds to send a migration request otherwise it will be aborted.
func (h *serviceMigrationHandler) AddIncomingMigration(details MigrationDetails) error {
	h.migrations_mu.Lock()
	defer h.migrations_mu.Unlock()

	if h.migrations[details.SystemJobID] != nil {
		return fmt.Errorf("migration for service %s already exists", details.SystemJobID)
	}

	details.lastUpdated = time.Now()
	h.migrations[details.SystemJobID] = &details
	log.Printf("Added incoming migration for service: %s to %s", details.SystemJobID, details.TargetNodeID)
	return nil
}

// Migrate performs the migration for a service to another node.
func (h *serviceMigrationHandler) Migrate(details MigrationDetails) error {

	// TODO: Check if the migration details are valid
	// TODO: Check if the service is running and can be migrated

	// Here you would implement the actual migration logic, such as transferring state, etc.
	log.Printf("Migrating service %s fto %s", details.SystemJobID, details.TargetNodeID)

	go func() {
		// Perform the migration request in a separate goroutine
		resp, err := h.requestMigration(details)
		if err != nil {
			log.Printf("Migration request failed for service %s: %v", details.SystemJobID, err)
			// Optionally, you can handle the error, such as logging it or notifying the user.
			return
		}
		log.Printf("Migration request successful for service %s: %s", details.SystemJobID, resp.GetMessage())
	}()

	return nil
}

// ReceiveMigration handles incoming migration data.
func (h *serviceMigrationHandler) ReceiveMigration(ctx context.Context, req *pb.MigrationData) (*pb.MigrationResponse, error) {
	log.Printf("Received migration for service: %s, payload size: %d", req.GetServiceId(), len(req.GetPayload()))

	h.migrations_mu.Lock()
	defer h.migrations_mu.Unlock()

	if h.migrations[req.GetServiceId()] == nil {
		log.Printf("No migration request found for service: %s", req.GetServiceId())
		return &pb.MigrationResponse{
			Success: false,
			Message: "Service migration request not found for migration: " + req.GetServiceId(),
		}, nil
	}
	if h.migrations[req.GetServiceId()].MigrationToken != req.GetMigrationToken() {
		log.Printf(
			"Migration token mismatch for service: %s, expected: %s, received: %s",
			req.GetServiceId(),
			h.migrations[req.GetServiceId()].MigrationToken,
			req.GetMigrationToken(),
		)
		return &pb.MigrationResponse{
			Success: false,
			Message: "Migration token mismatch for service: " + req.GetServiceId(),
		}, nil
	}
	if !h.migrations[req.GetServiceId()].acknowledged {
		log.Printf("Migration not acknowledged for service: %s", req.GetServiceId())
		return &pb.MigrationResponse{
			Success: false,
			Message: "Migration not acknowledged: " + req.GetServiceId(),
		}, nil
	}

	h.migrations[req.GetServiceId()].dataTransferred = true
	h.migrations[req.GetServiceId()].lastUpdated = time.Now()

	log.Printf("Processing migration payload for service: %s", req.GetServiceId())
	//TODO: Add payload to the runtime
	//TODO: Implement the logic to handle the migration payload, such as deserializing it and starting the service.

	// Migration successful, remove from the map
	delete(h.migrations, req.GetServiceId())

	return &pb.MigrationResponse{
		Success: true,
		Message: "Migration ack",
	}, nil
}

// RequestMigration handles incoming migration requests. Acks the migration and waits for furhter data.
func (h *serviceMigrationHandler) RequestMigration(ctx context.Context, req *pb.MigrationRequest) (*pb.MigrationResponse, error) {
	log.Printf("Received migration for service: %s", req.GetServiceId())

	h.migrations_mu.Lock()
	defer h.migrations_mu.Unlock()

	if h.migrations[req.GetServiceId()] == nil {
		log.Printf("No migration request found for service: %s", req.GetServiceId())
		return &pb.MigrationResponse{
			Success: false,
			Message: "Service migration request no found for migration: " + req.GetServiceId(),
		}, nil
	}
	if h.migrations[req.GetServiceId()].MigrationToken != req.GetMigrationToken() {
		log.Printf(
			"Migration token mismatch for service: %s, expected: %s, received: %s",
			req.GetServiceId(),
			h.migrations[req.GetServiceId()].MigrationToken,
			req.GetMigrationToken(),
		)
		return &pb.MigrationResponse{
			Success: false,
			Message: "Migration token mismatch for service: " + req.GetServiceId(),
		}, nil
	}

	h.migrations[req.GetServiceId()].acknowledged = true
	h.migrations[req.GetServiceId()].lastUpdated = time.Now()

	//TODO: Inform runtime for incoming migration
	log.Printf("Acknowledged migration request for service: %s", req.GetServiceId())

	return &pb.MigrationResponse{
		Success: true,
		Message: "Migration received successfully",
	}, nil
}

// AbortMigration handles aborting a migration process.
func (h *serviceMigrationHandler) AbortMigration(ctx context.Context, req *pb.MigrationRequest) (*pb.MigrationResponse, error) {
	log.Printf("Aborting migration for service: %s", req.GetServiceId())

	h.migrations_mu.Lock()
	defer h.migrations_mu.Unlock()

	if h.migrations[req.GetServiceId()] == nil {
		return &pb.MigrationResponse{
			Success: false,
			Message: "Service migration request no found for migration: " + req.GetServiceId(),
		}, nil
	}
	if h.migrations[req.GetServiceId()].MigrationToken != req.GetMigrationToken() {
		return &pb.MigrationResponse{
			Success: false,
			Message: "Migration token mismatch for service: " + req.GetServiceId(),
		}, nil
	}
	if h.migrations[req.GetServiceId()].dataTransferred {
		return &pb.MigrationResponse{
			Success: false,
			Message: "Migration data already transferred for service: " + req.GetServiceId(),
		}, nil
	}

	// Remove the migration from the map
	delete(h.migrations, req.GetServiceId())
	log.Printf("Migration aborted for service: %s", req.GetServiceId())

	return &pb.MigrationResponse{
		Success: true,
		Message: "Migration aborted successfully",
	}, nil
}

// RequestMigration acts as a client to request migration from another node.
// This function can be run as goroutine to handle migrations concurrently.
func (h *serviceMigrationHandler) requestMigration(migration MigrationDetails) (*pb.MigrationResponse, error) {
	// PHASE 1: Connect to the target node's migration server
	conn, err := grpc.NewClient(
		fmt.Sprintf("%s:%s", migration.TargetNodeIP, migration.TargetNodePort),
		grpc.WithMaxCallAttempts(5),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(
			grpc.ConnectParams{
				Backoff: backoff.Config{
					BaseDelay:  200 * time.Millisecond,
					Multiplier: 1.6,
				},
				MinConnectTimeout: 5 * time.Second,
			}),
		//TODO: use token for authentication
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to migration server on node %s at address %s, erro: %v", migration.TargetNodeID, migration.TargetNodeIP, err)
	}
	defer conn.Close()
	remote := pb.NewMigrationServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// PHASE 2: Verify migration pre-conditions
	//TODO: Implement logic to verify if the service can be migrated, such as checking if the service is running, etc.

	// PHASE 3: Request migration to target node and expect an acknowledgment
	attempt := 0
	for {
		attempt++
		if attempt > 5 {
			return nil, fmt.Errorf("migration request failed after 5 attempts to node %s", migration.TargetNodeIP)
		}
		log.Printf("Attempting migration request to node %s, attempt %d", migration.TargetNodeIP, attempt)
		response, err := remote.RequestMigration(ctx, &pb.MigrationRequest{
			ServiceId:      migration.SystemJobID,
			MigrationToken: migration.MigrationToken,
		})
		if err != nil {
			//TODO: add status migration failed in application status details
			return nil, fmt.Errorf("failed to request migration on node %s, error: %v", migration.TargetNodeID, err)
		}
		if response.GetSuccess() {
			//TODO: add status migration failed in application status details
			log.Printf("Migration request acknowledged by node %s for service %s", migration.TargetNodeID, migration.SystemJobID)
			break
		} else {
			log.Printf("Migration request failed on node %s for service %s, error:%s, retrying...", migration.TargetNodeID, migration.SystemJobID, response.GetMessage())
			// Optionally, you can add a sleep here to avoid hammering the server
			time.Sleep(1 * time.Second) // Wait before retrying
			continue
		}
	}

	// PHASE 4: Fetch runtime and get the payload
	// TODO: Implement logic to fetch the payload from the service being migrated
	payload := []byte("example payload") // Replace with actual payload fetching logic
	err = nil
	if err != nil {
		remote.MigrationAbort(ctx, &pb.MigrationRequest{
			ServiceId:      migration.SystemJobID,
			MigrationToken: migration.MigrationToken,
		})
		//TODO: add status migration failed in application status details
		return nil, fmt.Errorf("failed to fetch payload for migration: %v, MIGRATION ABORTED", err)
	}

	// PHASE 5: Send migration request
	req := &pb.MigrationData{
		ServiceId:      migration.SystemJobID,
		MigrationToken: migration.MigrationToken,
		Payload:        payload,
	}
	resp, err := remote.ReceiveMigration(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("migration request failed: %v", err)
		// TODO: add migration interruped and retry/rollback logic
	}
	return resp, nil
}
