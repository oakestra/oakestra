package csi

import (
	"context"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/logger"
	"sync"
	"time"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Plugin holds a live gRPC connection to a CSI Node plugin together with its
// discovered metadata and capabilities.
type Plugin struct {
	// Config is the static configuration entry from the Node Engine conf file.
	Config config.CSIDriverType

	// DriverName is the canonical name returned by GetPluginInfo.
	DriverName string

	// DriverVersion is the version string returned by GetPluginInfo.
	DriverVersion string

	// StageUnstageRequired reports whether the plugin advertises the
	// STAGE_UNSTAGE_VOLUME node capability (requires a two-step mount).
	StageUnstageRequired bool

	conn       *grpc.ClientConn
	identity   csipb.IdentityClient
	nodeClient csipb.NodeClient
}

// identityRPCClient returns the cached CSI Identity service client.
func (p *Plugin) identityRPCClient() csipb.IdentityClient { return p.identity }

// nodeRPCClient returns the cached CSI Node service client.
func (p *Plugin) nodeRPCClient() csipb.NodeClient { return p.nodeClient }

// Close releases the gRPC connection to the plugin.
func (p *Plugin) Close() {
	if p.conn != nil {
		_ = p.conn.Close()
	}
}

// Registry manages all currently active CSI plugins on this node.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin // keyed by driver name
}

var (
	registryOnce sync.Once
	globalReg    *Registry
)

// GetRegistry returns the process-wide CSI plugin registry (lazily initialised).
func GetRegistry() *Registry {
	registryOnce.Do(func() {
		globalReg = &Registry{
			plugins: make(map[string]*Plugin),
		}
	})
	return globalReg
}

// InitFromConfig reads CSI driver entries from the Node Engine configuration,
// probes each plugin, and registers the healthy ones. Call once at startup.
func (r *Registry) InitFromConfig(conf config.ConfFile) {
	for _, driverConf := range conf.CSIDrivers {
		if err := r.Register(driverConf); err != nil {
			logger.ErrorLogger().Printf("[CSI] Failed to register plugin %q (%s): %v",
				driverConf.Name, driverConf.Endpoint, err)
		}
	}
}

// Register probes a single CSI plugin endpoint and, on success, adds it to the
// registry.
func (r *Registry) Register(cfg config.CSIDriverType) error {
	logger.InfoLogger().Printf("[CSI] Probing plugin at %s", cfg.Endpoint)

	conn, err := dialPlugin(cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("dial %s: %w", cfg.Endpoint, err)
	}

	p := &Plugin{
		Config:     cfg,
		conn:       conn,
		identity:   csipb.NewIdentityClient(conn),
		nodeClient: csipb.NewNodeClient(conn),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Identity: Probe
	_, err = p.identity.Probe(ctx, &csipb.ProbeRequest{})
	if err != nil {
		conn.Close()
		return fmt.Errorf("Probe failed: %w", err)
	}

	// Identity: GetPluginInfo
	info, err := p.identity.GetPluginInfo(ctx, &csipb.GetPluginInfoRequest{})
	if err != nil {
		conn.Close()
		return fmt.Errorf("GetPluginInfo failed: %w", err)
	}
	p.DriverName = info.GetName()
	p.DriverVersion = info.GetVendorVersion()
	logger.InfoLogger().Printf("[CSI] Plugin info: name=%s version=%s", p.DriverName, p.DriverVersion)

	// Use discovered name if config did not specify one
	if cfg.Name == "" {
		cfg.Name = p.DriverName
		p.Config = cfg
	}

	// Node: GetCapabilities
	capsResp, err := p.nodeClient.NodeGetCapabilities(ctx, &csipb.NodeGetCapabilitiesRequest{})
	if err != nil {
		conn.Close()
		return fmt.Errorf("NodeGetCapabilities failed: %w", err)
	}
	for _, cap := range capsResp.GetCapabilities() {
		if cap.GetRpc().GetType() == csipb.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME {
			p.StageUnstageRequired = true
			logger.InfoLogger().Printf("[CSI] Plugin %s requires STAGE_UNSTAGE_VOLUME", p.DriverName)
		}
	}

	r.mu.Lock()
	r.plugins[p.DriverName] = p
	r.mu.Unlock()

	logger.InfoLogger().Printf("[CSI] Plugin %s registered (stageUnstage=%v)",
		p.DriverName, p.StageUnstageRequired)
	return nil
}

// Get returns the registered plugin for driverName, or an error if not found.
func (r *Registry) Get(driverName string) (*Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[driverName]
	if !ok {
		return nil, fmt.Errorf("CSI driver %q is not registered on this node", driverName)
	}
	return p, nil
}

// List returns copies of all registered CSIDriverType descriptors.
func (r *Registry) List() []config.CSIDriverType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]config.CSIDriverType, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p.Config)
	}
	return result
}

// StopAll closes all open plugin connections.
func (r *Registry) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, p := range r.plugins {
		logger.InfoLogger().Printf("[CSI] Closing connection to plugin %s", name)
		p.Close()
	}
	r.plugins = make(map[string]*Plugin)
}

// dialPlugin creates a gRPC connection to a CSI plugin UNIX domain socket.
// Endpoint may be either "/path/to.sock" or "unix:///path/to.sock".
func dialPlugin(endpoint string) (*grpc.ClientConn, error) {
	target := endpoint
	if len(target) > 0 && target[0] == '/' {
		target = "unix://" + target
	}
	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
