package jobs

import (
	"context"
	"fmt"
	"go_node_engine/model"
	pb "go_node_engine/requests/proto" // Adjust import path if needed
	virtualization "go_node_engine/virtualization"
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

// MIGRATION_SELF_DESTRUCT_TIMEOUT defines the timeout for migration self-destruct.
const MIGRATION_SELF_DESTRUCT_TIMEOUT = 30 * time.Second

type MigrationDetails struct {
	model.Service          // Embedding the Service struct to include service details
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
	migrations                      map[string]*MigrationDetails // Maps service ID to current migration
	migrations_mu                   sync.Mutex
	server                          *grpc.Server // gRPC server instance
	statusChangeNotificationHandler func(service model.Service)
}

type HandlerOptions func(*serviceMigrationHandler)

// ServiceMigrationHandler is responsible for handling service migration oeprations.
type MigrationHandler interface {
	AddIncomingMigration(MigrationDetails) error // Adds details for an incoming migration from another node.
	Migrate(MigrationDetails) error              // Performs a migration to another node.
}

// Get a singleton instance of the migration handler.
// The first time this function is called, it initializes the migration handler with the optin WithStatusChangeNotificationHandler
func GetMigrationHandler(opts ...HandlerOptions) MigrationHandler {
	migrationServiceSingletonOnce.Do(func() {
		migrationServiceSingleton = &serviceMigrationHandler{
			migrations: make(map[string]*MigrationDetails),
		}
		for _, opt := range opts {
			opt(migrationServiceSingleton)
		}
		if migrationServiceSingleton.statusChangeNotificationHandler == nil {
			log.Fatalf("Status change notification handler is not set. Use WithStatusChangeNotificationHandler to set it.")
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

	if h.migrations[details.JobID] != nil {
		return fmt.Errorf("migration for service %s already exists", details.JobID)
	}

	// prepare instantiation
	migrationRuntime, err := virtualization.GetRuntimeMigration(model.RuntimeType(details.Runtime))
	if err != nil {
		log.Printf("Migration runtime not found for virtualization %s: %v", details.Runtime, err)
		return fmt.Errorf("migration runtime not found for virtualization %s: %v", details.Runtime, err)
	}
	//TODO: Add Service structure to migration gRPC details

	details.lastUpdated = time.Now()
	h.migrations[details.JobID] = &details
	log.Printf("Added incoming migration for service: %s to %s", details.JobID, details.TargetNodeID)

	// Start migration self-destruct if ReceiveMigration is not called within MIGRATION_SELF_DESTRUCT_TIMEOUT seconds
	go func(serviceID string) {
		time.Sleep(MIGRATION_SELF_DESTRUCT_TIMEOUT)
		h.migrations_mu.Lock()
		defer h.migrations_mu.Unlock()
		if h.migrations[serviceID] == nil {
			log.Printf("Migration self-destruct for service: %s, already removed", serviceID)
			return
		}
		if !h.migrations[serviceID].dataTransferred {
			log.Printf("Migration self-destruct for service: %s, no data transferred", serviceID)
			delete(h.migrations, serviceID)
			migrationRuntime.AbortMigration(h.migrations[serviceID].Service)
		}
	}(details.JobID)

	return nil
}

// Migrate performs the migration for a service to another node.
func (h *serviceMigrationHandler) Migrate(details MigrationDetails) error {

	// TODO: Check if the migration details are valid
	// TODO: Check if the service is running and can be migrated

	// Here you would implement the actual migration logic, such as transferring state, etc.
	log.Printf("Migrating service %s to %s", details.JobID, details.TargetNodeID)

	go func() {
		// Perform the migration request in a separate goroutine
		resp, err := h.requestMigration(details)
		if err != nil {
			log.Printf("Migration request failed for service %s: %v", details.JobID, err)
			// Optionally, you can handle the error, such as logging it or notifying the user.
			return
		}
		log.Printf("Migration request successful for service %s: %s", details.JobID, resp.GetMessage())
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

	// Initialize the migration runtime for the service
	// The runtime downloads the image and prepares the idle container for instantiation.
	migrationRuntime, err := virtualization.GetRuntimeMigration(model.RuntimeType(h.migrations[req.GetServiceId()].Runtime))
	if err != nil {
		log.Printf("Failed to get runtime migration for service: %s, error: %v", req.GetServiceId(), err)
		return &pb.MigrationResponse{
			Success: false,
			Message: "Failed to get runtime migration: " + req.GetServiceId(),
		}, nil
	}
	err = migrationRuntime.PrepareForInstantiantion(h.migrations[req.GetServiceId()].Service, h.statusChangeNotificationHandler)
	if err != nil {
		log.Printf("Failed to prepare for instantiation for service: %s, error: %v", req.GetServiceId(), err)
		return &pb.MigrationResponse{
			Success: false,
			Message: "Failed to prepare for instantiation: " + req.GetServiceId(),
		}, nil
	}

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
	virtualizationRuntime, err := virtualization.GetRuntimeMigration(model.RuntimeType(migration.Runtime))
	if err != nil {
		return nil, fmt.Errorf("failed to get virtualization runtime for migration: %v", err)
	}
	service, err := virtualizationRuntime.SetMigrationCandidate(migration.Sname, migration.Instance)
	if err != nil {
		return nil, fmt.Errorf("failed to set migration candidate for service %s: %v", migration.Sname, err)
	}

	// PHASE 3: Request migration to target node and expect an acknowledgment
	attempt := 0
	for {
		attempt++
		if attempt > 5 {
			return nil, fmt.Errorf("migration request failed after 5 attempts to node %s", migration.TargetNodeIP)
		}
		log.Printf("Attempting migration request to node %s, attempt %d", migration.TargetNodeIP, attempt)
		response, err := remote.RequestMigration(ctx, &pb.MigrationRequest{
			ServiceId:      migration.JobID,
			MigrationToken: migration.MigrationToken,
		})
		if err != nil {
			// If the migration request fails, remove the migration candidate, send out an error and return
			// The service will keep running as usual.
			virtualizationRuntime.RemoveMigrationCandidate(migration.Sname, migration.Instance)
			return nil, fmt.Errorf("failed to request migration on node %s, error: %v", migration.TargetNodeID, err)
		}
		if response.GetSuccess() {
			log.Printf("Migration request acknowledged by node %s for service %s", migration.TargetNodeID, migration.JobID)
			break
		} else {
			log.Printf("Migration request failed on node %s for service %s, error:%s, retrying...", migration.TargetNodeID, migration.JobID, response.GetMessage())
			// Optionally, you can add a sleep here to avoid hammering the server
			time.Sleep(1 * time.Second) // Wait before retrying
			continue
		}
	}

	revert := func() {
		service.Status = model.SERVICE_FAILED
		service.StatusDetail = fmt.Sprintf("Migration request failed: %v", err)
		// Notify the status change handler
		h.statusChangeNotificationHandler(service)
		// Abort the migration on the remote node
		remote.MigrationAbort(ctx, &pb.MigrationRequest{
			ServiceId:      migration.JobID,
			MigrationToken: migration.MigrationToken,
		})
	}

	// PHASE 4: Fetch runtime and get the payload
	// From this point on, the service is no longer responsibility of the current node.
	// If a failure happens, the service must be rescheduled by the orchestrator.
	payload, err := virtualizationRuntime.StopAndGetState(migration.Sname, migration.Instance)
	if err != nil {
		defer revert()
		return nil, fmt.Errorf("failed to fetch payload for migration: %v, MIGRATION ABORTED", err)
	}

	// PHASE 5: Send migration request
	req := &pb.MigrationData{
		ServiceId:      migration.JobID,
		MigrationToken: migration.MigrationToken,
		Payload:        payload,
	}
	resp, err := remote.ReceiveMigration(ctx, req)
	if err != nil {
		defer revert()
		return nil, fmt.Errorf("migration request failed: %v", err)
	}
	return resp, nil
}

// WithStatusChangeNotificationHandler sets the handler for status change notifications.
// This handler is called when the status of a service changes during migration.
func WithStatusChangeNotificationHandler(notifyHandler func(service model.Service)) HandlerOptions {
	return func(h *serviceMigrationHandler) {
		h.statusChangeNotificationHandler = notifyHandler
	}
}
