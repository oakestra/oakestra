package cmd

import (
	"errors"
	"fmt"
	"go_node_engine/config"
	"os/exec"

	"github.com/spf13/cobra"
)

// P2P config-subcommand flags
var (
	modeP2PInit   bool
	modeP2PJoin   string
	modeP2PToken  string
	modeP2PCaHash string
)

func init() {
	// nested under the existing `config` command group
	configCmd.AddCommand(modeCmd)
	configCmd.AddCommand(schedCmd)
	modeCmd.AddCommand(modeP2PCmd)
	modeCmd.AddCommand(modeClusterCmd)
	modeCmd.AddCommand(modeShowCmd)

	modeP2PCmd.Flags().BoolVar(&modeP2PInit, "init", false, "Genesis: create a brand-new p2p network")
	modeP2PCmd.Flags().StringVar(&modeP2PJoin, "join", "", "Enrollment bootstrap address ip:port")
	modeP2PCmd.Flags().StringVar(&modeP2PToken, "token", "", "Single-use join token")
	modeP2PCmd.Flags().StringVar(&modeP2PCaHash, "ca-hash", "", "Trust-anchor pin (sha256:...)")

	modeClusterCmd.Flags().IntVarP(&clusterPort, "port", "p", 10100, "Cluster orchestrator port")

	schedCmd.Flags().IntVar(&schedSigma, "sigma", 0, "Bid quorum divisor SIGMA (default 4)")
	schedCmd.Flags().IntVar(&schedTimeout, "timeout", 0, "Bid-round timeout in seconds (default 30)")
}

// `config sched` — tune the bid-round quorum/timeout.
var (
	schedSigma   int
	schedTimeout int
	schedCmd     = &cobra.Command{
		Use:   "sched",
		Short: "Show or tune the p2p bid-round parameters (SIGMA, timeout)",
		RunE: func(cmd *cobra.Command, args []string) error {
			conf, err := config.Read()
			if err != nil {
				return err
			}
			if schedSigma > 0 {
				conf.P2P.SchedSigma = schedSigma
			}
			if schedTimeout > 0 {
				conf.P2P.SchedTimeoutSec = schedTimeout
			}
			if schedSigma > 0 || schedTimeout > 0 {
				if err := config.Write(conf); err != nil {
					return err
				}
			}
			fmt.Printf("Scheduler: SIGMA=%d timeout=%ds retry-cap=%ds replica-factor=%d\n",
				conf.P2P.SchedSigma, conf.P2P.SchedTimeoutSec, conf.P2P.SchedRetryMaxSec, conf.P2P.ReplicaFactor)
			return nil
		},
	}
)

var (
	modeCmd = &cobra.Command{
		Use:   "mode",
		Short: "Switch the worker between cluster and p2p mode",
	}
	modeP2PCmd = &cobra.Command{
		Use:   "p2p",
		Short: "Configure the worker for peer-to-peer mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return applyP2PStartup(modeP2PInit, modeP2PJoin, modeP2PToken, modeP2PCaHash)
		},
	}
	modeClusterCmd = &cobra.Command{
		Use:   "cluster [cluster-ip]",
		Short: "Configure the worker to attach to a cluster orchestrator",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return applyClusterMode(args[0], clusterPort)
		},
	}
	modeShowCmd = &cobra.Command{
		Use:   "show",
		Short: "Show the currently configured mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showMode()
		},
	}
)

// applyP2PStartup persists p2p mode + join parameters into the config file and
// ensures the node has a stable NodeUUID. Actual enrollment happens
// in the daemon / NetManager when it boots with mode=p2p.
func applyP2PStartup(initNet bool, join, token, caHash string) error {
	if !initNet && join == "" {
		// Neither genesis nor a join target: only valid if the node is already enrolled.
		// We allow it (bare re-join with stored creds) but warn on a fresh node below.
	}
	if join != "" && (token == "" || caHash == "") {
		return errors.New("--join requires both --token and --ca-hash")
	}
	if initNet && join != "" {
		return errors.New("--init and --join are mutually exclusive")
	}

	conf, err := config.Read()
	if err != nil {
		return err
	}

	// Switching FROM cluster mode drains first: cluster-scheduled workloads
	// belong to the orchestrator; warn the operator to undeploy them at the root too.
	switchingFromCluster := conf.NodeModeOrDefault() == config.MODE_CLUSTER && conf.ClusterAddress != "0.0.0.0"
	if switchingFromCluster {
		fmt.Println("Switching cluster → p2p: locally-persisted deployments will not be restored.")
		fmt.Println("Remember to undeploy this node's services at the root orchestrator as well.")
	}

	conf.Mode = config.MODE_P2P
	if conf.P2P.GossipPort == 0 {
		conf.P2P = config.GenDefaultP2PConfig()
	}

	// Stable identity for the p2p mesh (replaces the cluster-assigned id).
	if conf.NodeUUID == "" {
		uuid, err := config.GenerateNodeUUID()
		if err != nil {
			return err
		}
		conf.NodeUUID = uuid
	}

	// Stash join parameters so the daemon can run enrollment on boot. They are
	// one-shot: cleared after a successful enrollment. (Gossip seeds come from the
	// enrollment response — the --join address is the ENROLL endpoint, not a seed.)
	conf.P2P.Pending = config.PendingEnroll{
		Init:   initNet,
		Join:   join,
		Token:  token,
		CaHash: caHash,
	}

	if err := config.Write(conf); err != nil {
		return err
	}
	fmt.Printf("Worker configured for P2P mode 🟢 (node %s)\n", conf.NodeUUID)
	return nil
}

func applyClusterMode(address string, port int) error {
	conf, err := config.Read()
	if err != nil {
		return err
	}
	// Mode switch drains workloads first: the IP-allocation authority and discovery
	// mechanism change, so old instances would leave stale overlay state.
	if conf.NodeModeOrDefault() == config.MODE_P2P {
		drainBeforeModeSwitch()
	}
	conf.Mode = config.MODE_CLUSTER
	conf.ClusterAddress = address
	conf.ClusterPort = port
	if err := config.Write(conf); err != nil {
		return err
	}
	restartWorkerServices()
	fmt.Printf("Worker configured for cluster mode 🟢 (cluster %s:%d)\n", address, port)
	return nil
}

// drainBeforeModeSwitch best-effort undeploys everything via the running daemon
// If the daemon is down there is nothing running to drain.
func drainBeforeModeSwitch() {
	var views []map[string]interface{}
	if err := ctrlGet("/ctrl/services", &views); err != nil {
		return
	}
	for _, v := range views {
		if l, ok := v["local"].(bool); ok && l {
			job, _ := v["job_name"].(string)
			instance := 0
			if i, ok := v["instance"].(float64); ok {
				instance = int(i)
			}
			if err := ctrlDelete(fmt.Sprintf("/ctrl/services/%s/%d", job, instance)); err == nil {
				fmt.Printf("drained %s.%d\n", job, instance)
			}
		}
	}
}

// restartWorkerServices applies the new mode by restarting both daemons.
func restartWorkerServices() {
	_ = exec.Command("systemctl", "restart", "netmanager").Run()
	_ = exec.Command("systemctl", "restart", "nodeengine").Run()
}

func showMode() error {
	conf, err := config.Read()
	if err != nil {
		return err
	}
	switch conf.NodeModeOrDefault() {
	case config.MODE_P2P:
		fmt.Printf("Mode: p2p 🟢\n  node_uuid: %s\n  gossip_port: %d\n  enroll_port: %d\n",
			conf.NodeUUID, conf.P2P.GossipPort, conf.P2P.EnrollPort)
	default:
		fmt.Printf("Mode: cluster 🟢\n  cluster: %s:%d\n", conf.ClusterAddress, conf.ClusterPort)
	}
	return nil
}

func dedupeAppend(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}
