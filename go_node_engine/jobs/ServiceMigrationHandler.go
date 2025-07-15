package jobs

import (
	"context"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	pb "go_node_engine/requests/proto" // Adjust import path if needed
	virtualization "go_node_engine/virtualization"
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
			logger.InfoLogger().Fatalf("Status change notification handler is not set. Use WithStatusChangeNotificationHandler to set it.")
		}
		go func() {
			err := migrationServiceSingleton.startMigrationServer(
				fmt.Sprintf("[::]:%s", model.GetNodeInfo().Port), // Replace with your desired address and port
			)
			if err != nil {
				logger.InfoLogger().Fatalf("Failed to start migration server: %v", err)
			}
		}()
	})
	return migrationServiceSingleton
}

// StopMigrationHandler stops the migration server gracefully.
func StopMigrationHandler() {
	migrationServiceSingleton.server.GracefulStop()
	logger.InfoLogger().Println("Migration server stopped gracefully")
}

// StartMigrationServer starts the gRPC server to receive migrations.
func (h *serviceMigrationHandler) startMigrationServer(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	h.server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	pb.RegisterMigrationServiceServer(h.server, h)
	logger.InfoLogger().Printf("Migration server listening at %s", address)
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
		logger.ErrorLogger().Printf("Migration runtime not found for virtualization %s: %v", details.Runtime, err)
		return fmt.Errorf("migration runtime not found for virtualization %s: %v", details.Runtime, err)
	}
	//TODO: Add Service structure to migration gRPC details

	details.lastUpdated = time.Now()
	h.migrations[details.JobID] = &details
	logger.InfoLogger().Printf("Added incoming migration for service: %s to %s", details.JobID, details.TargetNodeID)

	// Start migration self-destruct if ReceiveMigration is not called within MIGRATION_SELF_DESTRUCT_TIMEOUT seconds
	go func(serviceID string) {
		time.Sleep(MIGRATION_SELF_DESTRUCT_TIMEOUT)
		h.migrations_mu.Lock()
		defer h.migrations_mu.Unlock()
		if val, exist := h.migrations[serviceID]; !exist || val == nil {
			// Migration already removed, no need to self-destruct
			logger.ErrorLogger().Printf("Migration self-destruct for service: %s, already removed", serviceID)
			return
		}
		if !h.migrations[serviceID].dataTransferred {
			logger.ErrorLogger().Printf("Migration self-destruct for service: %s, no data transferred", serviceID)
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
	logger.InfoLogger().Printf("Migrating service %s to %s", details.JobID, details.TargetNodeID)

	go func() {
		// Perform the migration request in a separate goroutine
		resp, err := h.requestMigration(details)
		if err != nil {
			logger.ErrorLogger().Printf("Migration request failed for service %s: %v", details.JobID, err)
			// Optionally, you can handle the error, such as logging it or notifying the user.
			return
		}
		logger.InfoLogger().Printf("Migration request successful for service %s: %s", details.JobID, resp.GetMessage())
	}()

	return nil
}

// ReceiveMigration handles incoming migration data.
func (h *serviceMigrationHandler) ReceiveMigration(ctx context.Context, req *pb.MigrationData) (*pb.MigrationResponse, error) {
	logger.InfoLogger().Printf("Received migration for service: %s, payload size: %d", req.GetServiceId(), len(req.GetPayload()))

	h.migrations_mu.Lock()
	defer h.migrations_mu.Unlock()

	if h.migrations[req.GetServiceId()] == nil {
		logger.ErrorLogger().Printf("No migration request found for service: %s", req.GetServiceId())
		return &pb.MigrationResponse{
			Success: false,
			Message: "Service migration request not found for migration: " + req.GetServiceId(),
		}, nil
	}
	if h.migrations[req.GetServiceId()].MigrationToken != req.GetMigrationToken() {
		logger.ErrorLogger().Printf(
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
		logger.ErrorLogger().Printf("Migration not acknowledged for service: %s", req.GetServiceId())
		return &pb.MigrationResponse{
			Success: false,
			Message: "Migration not acknowledged: " + req.GetServiceId(),
		}, nil
	}

	h.migrations[req.GetServiceId()].dataTransferred = true
	h.migrations[req.GetServiceId()].lastUpdated = time.Now()

	logger.InfoLogger().Printf("Processing migration payload for service: %s", req.GetServiceId())
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
	logger.InfoLogger().Printf("Received migration for service: %s", req.GetServiceId())

	h.migrations_mu.Lock()
	defer h.migrations_mu.Unlock()

	if h.migrations[req.GetServiceId()] == nil {
		logger.ErrorLogger().Printf("No migration request found for service: %s", req.GetServiceId())
		return &pb.MigrationResponse{
			Success: false,
			Message: "Service migration request no found for migration: " + req.GetServiceId(),
		}, nil
	}
	if h.migrations[req.GetServiceId()].MigrationToken != req.GetMigrationToken() {
		logger.ErrorLogger().Printf(
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
		logger.ErrorLogger().Printf("Failed to get runtime migration for service: %s, error: %v", req.GetServiceId(), err)
		return &pb.MigrationResponse{
			Success: false,
			Message: "Failed to get runtime migration: " + req.GetServiceId(),
		}, nil
	}
	err = migrationRuntime.PrepareForInstantiantion(h.migrations[req.GetServiceId()].Service, h.statusChangeNotificationHandler)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to prepare for instantiation for service: %s, error: %v", req.GetServiceId(), err)
		return &pb.MigrationResponse{
			Success: false,
			Message: "Failed to prepare for instantiation: " + req.GetServiceId(),
		}, nil
	}

	logger.InfoLogger().Printf("Acknowledged migration request for service: %s", req.GetServiceId())
	return &pb.MigrationResponse{
		Success: true,
		Message: "Migration received successfully",
	}, nil
}

// MigrationAbort handles aborting a migration process.
func (h *serviceMigrationHandler) MigrationAbort(ctx context.Context, req *pb.MigrationRequest) (*pb.MigrationResponse, error) {
	logger.InfoLogger().Printf("Aborting migration for service: %s", req.GetServiceId())

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
	migrationRuntime, err := virtualization.GetRuntimeMigration(model.RuntimeType(h.migrations[req.GetServiceId()].Runtime))
	if err != nil {
		logger.ErrorLogger().Printf("Failed to get runtime migration for service: %s, error: %v", req.GetServiceId(), err)
		return &pb.MigrationResponse{
			Success: false,
			Message: "Failed to get runtime migration: " + req.GetServiceId(),
		}, nil
	}
	err = migrationRuntime.AbortMigration(h.migrations[req.GetServiceId()].Service)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to abort migration for service: %s, error: %v", req.GetServiceId(), err)
		return &pb.MigrationResponse{
			Success: false,
			Message: "Failed to abort migration: " + req.GetServiceId(),
		}, nil
	}
	logger.InfoLogger().Printf("Migration aborted for service: %s", req.GetServiceId())

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
	_, err = virtualizationRuntime.SetMigrationCandidate(migration.Sname, migration.Instance)
	if err != nil {
		return nil, fmt.Errorf("failed to set migration candidate for service %s: %v", migration.Sname, err)
	}

	// PHASE 3: Request migration to target node and expect an acknowledgment
	attempt := 0
	for {
		attempt++
		if attempt > 5 {
			virtualizationRuntime.RemoveMigrationCandidate(migration.Sname, migration.Instance)
			return nil, fmt.Errorf("migration request failed after 5 attempts to node %s", migration.TargetNodeIP)
		}
		logger.InfoLogger().Printf("Attempting migration request to node %s, attempt %d", migration.TargetNodeIP, attempt)
		response, err := remote.RequestMigration(ctx, &pb.MigrationRequest{
			ServiceId:      migration.JobID,
			MigrationToken: migration.MigrationToken,
		})
		if err != nil {
			logger.ErrorLogger().Printf("Migration request failed on node %s for service %s, error:%s, retrying...", migration.TargetNodeID, migration.JobID, err)
			time.Sleep(1 * time.Second) // Wait before retrying
			continue
		}
		if response.GetSuccess() {
			logger.InfoLogger().Printf("Migration request acknowledged by node %s for service %s", migration.TargetNodeID, migration.JobID)
			break
		} else {
			logger.ErrorLogger().Printf("Migration request not successfull on node %s for service %s, error:%s, retrying...", migration.TargetNodeID, migration.JobID, response.GetMessage())
			time.Sleep(1 * time.Second) // Wait before retrying
			continue
		}
	}

	revertMigrationAbort := func() {
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
		defer revertMigrationAbort()
		return nil, fmt.Errorf("failed to fetch payload for migration: %v, MIGRATION ABORTED", err)
	}

	// PHASE 5: Send migration request, from now on the service is no longer responsibility of the current node.
	req := &pb.MigrationData{
		ServiceId:      migration.JobID,
		MigrationToken: migration.MigrationToken,
		Payload:        payload,
	}
	resp, err := remote.ReceiveMigration(ctx, req)
	if err != nil {
		defer revertMigrationAbort()
		defer h.reDeployIfMigrationFails(migration.Service, payload)
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

func (h *serviceMigrationHandler) reDeployIfMigrationFails(service model.Service, payload []byte) {
	virtualizationRuntime, err := virtualization.GetRuntimeMigration(model.RuntimeType(service.Runtime))
	if err != nil {
		logger.ErrorLogger().Printf("Failed to get virtualization runtime for re-deployment: %v", err)
		return
	}

	time.Sleep(5 * time.Second) // Wait for cooldown before re-deploying

	err = virtualizationRuntime.PrepareForInstantiantion(service, h.statusChangeNotificationHandler)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to prepare for instantiation after migration failure: %v", err)
		return
	}
	err = virtualizationRuntime.ResumeFromState(service.Sname, service.Instance, payload, h.statusChangeNotificationHandler)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to re-deploy service %s after migration failure: %v", service.Sname, err)
		return
	}
	logger.InfoLogger().Printf("Service %s re-deployed successfully after migration failure", service.Sname)
	service.Status = model.SERVICE_RUNNING
	h.statusChangeNotificationHandler(service)
}
