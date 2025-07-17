package jobs

import (
	"context"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	pb "go_node_engine/requests/proto" // Adjust import path if needed
	"go_node_engine/utils"
	virtualization "go_node_engine/virtualization"
	"io"
	"net"
	"os"
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
			delete(h.migrations, serviceID)
			logger.InfoLogger().Printf("Migration self-destruct for service: %s, migration aborted", serviceID)
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
func (h *serviceMigrationHandler) ReceiveMigration(stream pb.MigrationService_ReceiveMigrationServer) error {
	logger.InfoLogger().Println("Received migration data stream")

	// create model.GetNodeInfo().CheckpointDirectory
	os.MkdirAll(model.GetNodeInfo().CheckpointDirectory, 0755)
	stateFileName := fmt.Sprintf("%s/%s.checkpoint.tar.gz", model.GetNodeInfo().CheckpointDirectory, utils.GenerateRandomString(16))
	stateFile, err := os.Create(stateFileName) // Create the file to store the migration state
	if err != nil {
		logger.ErrorLogger().Printf("Failed to create state file for migration: %v", err)
		stream.SendAndClose(&pb.MigrationResponse{
			Success: false,
			Message: "Failed to create state file for migration: " + err.Error(),
		})
		return err
	}
	totWritten := 0

	for {
		data, err := stream.Recv()
		if err != nil && err != io.EOF {
			os.Remove(stateFileName)
			return err
		}

		rsp, err := h.checkReceivedMigrationDataChunk(data)
		if err != nil {
			stream.SendAndClose(rsp)
			os.Remove(stateFileName) // Clean up the state file on error
			return err
		}

		// Write the received data to the state file
		if len(data.GetPayload()) > 0 {
			n, writeErr := stateFile.WriteAt(data.GetPayload(), int64(totWritten))
			if writeErr != nil || n != len(data.GetPayload()) {
				logger.ErrorLogger().Printf("Failed to write migration data to file: %v", writeErr)
				os.Remove(stateFileName) // Clean up the state file on error
				stream.SendAndClose(&pb.MigrationResponse{
					Success: false,
					Message: "Failed to write migration data to file: " + writeErr.Error(),
				})
				return writeErr
			}
			totWritten += n
		}

		if err == io.EOF || (data != nil && data.IsFinal) {
			logger.InfoLogger().Printf("Received all migration data for service: %s, total bytes written: %d", data.GetServiceId(), totWritten)
			h.migrations_mu.Lock()
			defer h.migrations_mu.Unlock()
			h.migrations[data.GetServiceId()].dataTransferred = true
			h.migrations[data.GetServiceId()].lastUpdated = time.Now()
			sname := h.migrations[data.GetServiceId()].Sname
			instance := h.migrations[data.GetServiceId()].Instance

			migrationRuntime, err := virtualization.GetRuntimeMigration(model.RuntimeType(h.migrations[data.GetServiceId()].Runtime))
			if err != nil {
				logger.ErrorLogger().Printf("Failed to get migration runtime: %v", err)
				os.Remove(stateFileName) // Clean up the state file on error
				stream.SendAndClose(&pb.MigrationResponse{
					Success: false,
					Message: "Failed to write migration data to file: " + err.Error(),
				})
				return err
			}
			// Resume the service from the state file asyncrhonously
			go func() {
				err := migrationRuntime.ResumeFromState(sname, instance, stateFileName, h.statusChangeNotificationHandler)
				if err != nil {
					logger.ErrorLogger().Printf("Failed to resume migration for service: %s, error: %v", data.GetServiceId(), err)
				}
			}()

			// Migration successful, remove from the map
			delete(h.migrations, data.GetServiceId())

			return stream.SendAndClose(&pb.MigrationResponse{
				Success: true,
			})
		}

	}
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

	// get the application state
	stateReader, err := virtualizationRuntime.StopAndGetState(migration.Sname, migration.Instance)
	if err != nil {
		defer revertMigrationAbort()
		return nil, fmt.Errorf("failed to fetch payload for migration: %v, MIGRATION ABORTED", err)
	}

	// PHASE 5: Send migration request, from now on the service is no longer responsibility of the current node.
	// Read the state data from the OnceReader
	migrationStreamingInterface, err := remote.ReceiveMigration(ctx)
	if err != nil {
		defer revertMigrationAbort()
		defer h.reDeployIfMigrationFails(migration.Service, stateReader)
		return nil, fmt.Errorf("migration request failed: %v", err)
	}

	buffer := make([]byte, 1024*1024) // 1MB buffer
	sendData := pb.MigrationData{
		ServiceId:      migration.JobID,
		MigrationToken: migration.MigrationToken,
		Payload:        nil,
		IsFinal:        false, // This will be set to true when the last chunk is sent
	}

	for {
		n, readErr := stateReader.Read(buffer)

		if n > 0 {
			sendData.Payload = buffer[:n]
		} else {
			sendData.IsFinal = true
		}
		if readErr == io.EOF {
			sendData.IsFinal = true // Mark the last chunk
		}
		err = migrationStreamingInterface.Send(&sendData)
		if err != nil {
			defer revertMigrationAbort()
			defer h.reDeployIfMigrationFails(migration.Service, stateReader)
			return nil, fmt.Errorf("failed to send migration data: %v", err)
		}

		if sendData.IsFinal {
			logger.InfoLogger().Printf("Sent all migration data for service: %s, total bytes sent: %d", migration.JobID, n)
			stateReader.Delete()
			break
		}
		if readErr != nil && readErr != io.EOF {
			defer revertMigrationAbort()
			stateReader.Delete() // Clean up the state file on error
			return nil, fmt.Errorf("failed to get payload for migration: %v, MIGRATION ABORTED", readErr)
		}
	}

	return migrationStreamingInterface.CloseAndRecv()
}

// WithStatusChangeNotificationHandler sets the handler for status change notifications.
// This handler is called when the status of a service changes during migration.
func WithStatusChangeNotificationHandler(notifyHandler func(service model.Service)) HandlerOptions {
	return func(h *serviceMigrationHandler) {
		h.statusChangeNotificationHandler = notifyHandler
	}
}

func (h *serviceMigrationHandler) reDeployIfMigrationFails(service model.Service, stateReader utils.OnceReader) {
	defer stateReader.Delete() // Ensure the state file is deleted after re-deployment

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
	err = virtualizationRuntime.ResumeFromState(service.Sname, service.Instance, stateReader.GetFile().Name(), h.statusChangeNotificationHandler)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to re-deploy service %s after migration failure: %v", service.Sname, err)
		return
	}
	logger.InfoLogger().Printf("Service %s re-deployed successfully after migration failure", service.Sname)
	service.Status = model.SERVICE_RUNNING
	h.statusChangeNotificationHandler(service)
}

func (h *serviceMigrationHandler) checkReceivedMigrationDataChunk(req *pb.MigrationData) (*pb.MigrationResponse, error) {
	logger.InfoLogger().Printf("Received migration chunk for service: %s, payload size: %d", req.GetServiceId(), len(req.GetPayload()))

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

	return &pb.MigrationResponse{
		Success: true,
		Message: "Migration data chunk received successfully",
	}, nil
}
