package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/model"
	p2pnode "go_node_engine/p2p"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

// P2P application lifecycle CLI: thin clients to the local node control API.
// Only meaningful in p2p mode; in cluster mode they refuse and point at the root API.

const controlPort = 50105

var (
	deployLocal bool
	logsTail    int

	deployCmd = &cobra.Command{
		Use:   "deploy [sla.json]",
		Short: "Schedule SLA service(s) across the p2p network (or locally with --local)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			sla, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]interface{}{
				"sla":   json.RawMessage(sla),
				"local": deployLocal,
			})
			var results []map[string]interface{}
			if err := ctrlPost("/ctrl/deploy", payload, &results); err != nil {
				return err
			}
			for _, r := range results {
				line := fmt.Sprintf("%v: %v", r["job_name"], r["status"])
				if host, ok := r["host"].(string); ok && host != "" {
					line += fmt.Sprintf(" on %v", host)
				}
				if rr, ok := r["rr_ip"].(string); ok && rr != "" {
					line += fmt.Sprintf(" (ServiceIP %v)", rr)
				}
				if detail, ok := r["detail"].(string); ok && detail != "" {
					line += " — " + detail
				}
				fmt.Println(line)
			}
			return nil
		},
	}

	undeployCmd = &cobra.Command{
		Use:   "undeploy [job-name] [instance]",
		Short: "Stop a service and deregister it from the p2p network",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			instance := 0
			if len(args) == 2 {
				instance, _ = strconv.Atoi(args[1])
			}
			if err := ctrlDelete(fmt.Sprintf("/ctrl/services/%s/%d", args[0], instance)); err != nil {
				return err
			}
			fmt.Printf("%s undeployed 🟢\n", args[0])
			return nil
		},
	}

	servicesCmd = &cobra.Command{
		Use:   "services",
		Short: "Inspect services across the whole p2p network",
	}

	servicesLsCmd = &cobra.Command{
		Use:   "ls",
		Short: "List ALL services in the network (from the gossiped registry — instant)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			var views []map[string]interface{}
			if err := ctrlGet("/ctrl/services", &views); err != nil {
				return err
			}
			if len(views) == 0 {
				fmt.Println("No services in the network.")
				return nil
			}
			fmt.Printf("%-40s %-14s %-10s %-16s %-6s %s\n", "JOB", "HOST", "STATUS", "SERVICE-IP", "GEN", "LOCAL")
			for _, v := range views {
				local := ""
				if l, ok := v["local"].(bool); ok && l {
					local = "*"
				}
				fmt.Printf("%-40v %-14.14v %-10v %-16v %-6v %s\n",
					v["job_name"], v["host"], v["status"], v["rr_ip"], v["generation"], local)
			}
			return nil
		},
	}

	servicesLogsCmd = &cobra.Command{
		Use:   "logs [job-name] [instance]",
		Short: "Fetch app logs from the hosting node",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			instance := 0
			if len(args) == 2 {
				instance, _ = strconv.Atoi(args[1])
			}
			// Always ask the local controller: it serves local logs directly and proxies
			// to the hosting node otherwise — the kubectl-logs pattern.
			resp, err := ctrlDo(http.MethodGet, fmt.Sprintf("/ctrl/logs?job=%s&instance=%d&tail=%d", args[0], instance, logsTail), nil)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			fmt.Print(string(body))
			return nil
		},
	}

	servicesInspectCmd = &cobra.Command{
		Use:   "inspect [job-name]",
		Short: "Show the registry view of one service (host, IPs, status, generation)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			var views []map[string]interface{}
			if err := ctrlGet("/ctrl/services", &views); err != nil {
				return err
			}
			found := false
			for _, v := range views {
				if v["job_name"] == args[0] {
					data, _ := json.MarshalIndent(v, "", "  ")
					fmt.Println(string(data))
					found = true
				}
			}
			if !found {
				fmt.Printf("Service %s not found in the network registry.\n", args[0])
			}
			return nil
		},
	}

	membersCmd = &cobra.Command{
		Use:   "members",
		Short: "List the authenticated members of the p2p network",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			// members come from the NetManager's gossip view, proxied via the daemon config
			var members []map[string]interface{}
			if err := nmGet("/p2p/members", &members); err != nil {
				return err
			}
			fmt.Printf("%-40s %s\n", "NODE-UUID", "ADDRESS")
			for _, m := range members {
				fmt.Printf("%-40v %v\n", m["uuid"], m["addr"])
			}
			return nil
		},
	}

	capRuntime string
	capMemory  int
	capVcpus   int
	capVgpus   int
	capCSI     []string
	capJSON    bool

	membersCapacityCmd = &cobra.Command{
		Use:   "capacity [node-uuid]",
		Short: "Show every member's schedulable capacity, and whether it would bid for a job",
		Long: "Queries each member's control API for the resources its bid evaluation uses.\n" +
			"With --runtime/--memory/--vcpus/--vgpus/--csi, every member also evaluates that job with\n" +
			"its real bid logic and reports why it would decline.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true // flags parsed; errors from here on are not usage errors
			if err := requireP2PMode(); err != nil {
				return err
			}
			query := url.Values{"network": {"1"}}
			flags := cmd.Flags()
			if flags.Changed("runtime") || flags.Changed("memory") || flags.Changed("vcpus") || flags.Changed("vgpus") || flags.Changed("csi") {
				query.Set("runtime", capRuntime)
				query.Set("memory", strconv.Itoa(capMemory))
				query.Set("vcpus", strconv.Itoa(capVcpus))
				query.Set("vgpus", strconv.Itoa(capVgpus))
				for _, driver := range capCSI {
					query.Add("csi", driver)
				}
			}
			var network p2pnode.NetworkCapacity
			if err := ctrlGet("/ctrl/capacity?"+query.Encode(), &network); err != nil {
				return err
			}
			if len(args) == 1 {
				kept := network.Members[:0]
				for _, m := range network.Members {
					if strings.HasPrefix(m.NodeUUID, args[0]) {
						kept = append(kept, m)
					}
				}
				if len(kept) == 0 {
					return fmt.Errorf("no member matches %q", args[0])
				}
				network.Members = kept
			}
			if capJSON {
				data, _ := json.MarshalIndent(network, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			printCapacity(network, query.Has("runtime"))
			return nil
		},
	}

	tokenTTL time.Duration

	addp2pCmd = &cobra.Command{
		Use:   "addp2p",
		Short: "Onboard a new node: mint a single-use join token and print the copy-paste commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			conf, err := config.Read()
			if err != nil {
				return err
			}
			if _, ok := p2pnode.LoadCreds(); !ok {
				return fmt.Errorf("this node is not enrolled yet — start it first: NodeEngine --p2p -d")
			}
			token, err := p2pnode.MintToken(tokenTTL)
			if err != nil {
				return err
			}
			_, fp, err := p2pnode.EnsureIdentity(conf.NodeUUID)
			if err != nil {
				return err
			}
			enrollPort := conf.P2P.EnrollPort
			if enrollPort == 0 {
				enrollPort = 50106
			}
			myIP := model.GetNodeInfo().Ip

			// Install lines reproduced verbatim from today's flow; the join logic lives
			// entirely in the boot command — the installer is untouched.
			fmt.Printf("Run this on the new node (valid %s, single use):\n\n", tokenTTL)
			fmt.Printf("  # 1) install (existing, unchanged)\n")
			fmt.Printf("  curl -sfL oakestra.io/oak.sh | bash\n")
			fmt.Printf("  oak install worker\n\n")
			fmt.Printf("  # 2) boot + join the p2p network\n")
			fmt.Printf("  NodeEngine -d --p2p \\\n")
			fmt.Printf("      --join    %s:%d \\\n", myIP, enrollPort)
			fmt.Printf("      --token   %s \\\n", token)
			fmt.Printf("      --ca-hash %s\n", fp)
			return nil
		},
	}

	revokeCmd = &cobra.Command{
		Use:   "revoke [node-uuid]",
		Short: "Revoke a node: remove it from the roster and rotate the network keys",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			// Full revocation (roster removal + PSK/data-key rotation across all members,
			// is orchestrated by the running daemon.
			payload, _ := json.Marshal(map[string]string{"node_uuid": args[0]})
			if err := ctrlPost("/ctrl/revoke", payload, nil); err != nil {
				// Daemon down: at least remove local trust, keys rotate on next revoke.
				if lerr := p2pnode.Revoke(args[0]); lerr != nil {
					return fmt.Errorf("%v (and local roster removal failed: %v)", err, lerr)
				}
				fmt.Printf("%s removed from the local roster, but the daemon is down —\n", args[0])
				fmt.Println("network keys were NOT rotated. Start the daemon and re-run revoke.")
				return nil
			}
			fmt.Printf("%s revoked 🟢 — roster updated and network keys rotated on all members.\n", args[0])
			return nil
		},
	}

	maintenanceDur time.Duration

	drainCmd = &cobra.Command{
		Use:   "drain",
		Short: "Undeploy every service hosted on this node (used before mode switch / maintenance)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireP2PMode(); err != nil {
				return err
			}
			// --maintenance: instead of undeploying, announce a planned-downtime window so
			// peers suppress failover; a quick reboot then reclaims its own jobs.
			if maintenanceDur > 0 {
				payload, _ := json.Marshal(map[string]string{"duration": maintenanceDur.String()})
				if err := ctrlPost("/ctrl/maintenance", payload, nil); err != nil {
					return err
				}
				fmt.Printf("Maintenance window of %s announced 🟢 — peers will not reschedule this node's jobs.\n", maintenanceDur)
				fmt.Println("Reboot within the window and the node reclaims its own workloads.")
				return nil
			}
			var views []map[string]interface{}
			if err := ctrlGet("/ctrl/services", &views); err != nil {
				return err
			}
			drained := 0
			for _, v := range views {
				if l, ok := v["local"].(bool); ok && l {
					job, _ := v["job_name"].(string)
					instance := 0
					if i, ok := v["instance"].(float64); ok {
						instance = int(i)
					}
					if err := ctrlDelete(fmt.Sprintf("/ctrl/services/%s/%d", job, instance)); err == nil {
						fmt.Printf("drained %s.%d\n", job, instance)
						drained++
					}
				}
			}
			fmt.Printf("%d service(s) drained 🟢\n", drained)
			return nil
		},
	}
)

func init() {
	deployCmd.Flags().BoolVar(&deployLocal, "local", false, "Force placement on THIS node (skip sampling)")
	servicesLogsCmd.Flags().IntVar(&logsTail, "tail", 100, "Number of log lines to fetch")
	addp2pCmd.Flags().DurationVar(&tokenTTL, "ttl", time.Hour, "Join-token validity (e.g. 30m, 2h)")
	drainCmd.Flags().DurationVar(&maintenanceDur, "maintenance", 0,
		"Announce planned downtime instead of undeploying: peers suppress failover for this duration")

	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(undeployCmd)
	rootCmd.AddCommand(servicesCmd)
	rootCmd.AddCommand(membersCmd)
	rootCmd.AddCommand(addp2pCmd)
	rootCmd.AddCommand(revokeCmd)
	rootCmd.AddCommand(drainCmd)
	servicesCmd.AddCommand(servicesLsCmd)
	servicesCmd.AddCommand(servicesLogsCmd)
	servicesCmd.AddCommand(servicesInspectCmd)
	membersCmd.AddCommand(membersCapacityCmd)
	membersCapacityCmd.Flags().StringVar(&capRuntime, "runtime", "container", "SLA virtualization of the job to evaluate: container (alias docker), unikernel, crosvm, ...")
	membersCapacityCmd.Flags().IntVar(&capMemory, "memory", 0, "memory in MB of the job to evaluate")
	membersCapacityCmd.Flags().IntVar(&capVcpus, "vcpus", 0, "vcpus of the job to evaluate")
	membersCapacityCmd.Flags().IntVar(&capVgpus, "vgpus", 0, "vgpus of the job to evaluate")
	membersCapacityCmd.Flags().StringSliceVar(&capCSI, "csi", nil, "CSI driver(s) the job's volumes need")
	membersCapacityCmd.Flags().BoolVar(&capJSON, "json", false, "print the raw JSON response")
}

// printCapacity renders a network capacity sweep as a table. withBid adds the
// per-member verdict on the evaluated job.
func printCapacity(network p2pnode.NetworkCapacity, withBid bool) {
	header := fmt.Sprintf("%-36s %-18s %5s %6s %15s %4s %4s  %s", "NODE-UUID", "RUNTIMES", "VCPUS", "CPU%", "MEM-FREE/TOTAL", "GPUS", "SVCS", "CSI")
	if withBid {
		header += "  BID"
	}
	fmt.Println(header)
	bidders := 0
	for _, m := range network.Members {
		c := m.Capacity
		if c == nil {
			fmt.Printf("%-36s unreachable: %s\n", m.NodeUUID, m.Error)
			continue
		}
		csi := strings.Join(c.CSIDrivers, ",")
		if csi == "" {
			csi = "-"
		}
		line := fmt.Sprintf("%-36s %-18s %5d %6.1f %15s %4d %4d  %s",
			m.NodeUUID, strings.Join(c.Runtimes, ","), c.Vcpus, c.CpuPct,
			fmt.Sprintf("%.0f/%d MB", c.FreeMemMB, c.MemoryMB), c.Vgpus, c.Services, csi)
		if withBid && c.Bid != nil {
			if c.Bid.Ok {
				bidders++
				line += "  yes"
			} else {
				line += "  no: " + c.Bid.Reason
			}
		}
		fmt.Println(line)
	}
	fmt.Printf("\nA deploy asks %d random matching member(s) for room (probe timeout %ds), and the\n"+
		"other matching members only if none of them has room.\n", network.Sample, network.ProbeTimeoutSec)
	if withBid {
		fmt.Printf("%d of %d member(s) would bid for this job.\n", bidders, len(network.Members))
		if bidders == 0 {
			fmt.Println("No member would bid: a deploy would end UNSCHEDULABLE and be parked as PENDING.")
		}
	}
}

func requireP2PMode() error {
	conf, err := config.Read()
	if err != nil {
		return err
	}
	if conf.NodeModeOrDefault() != config.MODE_P2P {
		return fmt.Errorf("this command requires p2p mode — the worker is in cluster mode.\n" +
			"Use the root orchestrator API (or `oak` against the root) instead, or switch with:\n" +
			"  NodeEngine config mode p2p")
	}
	return nil
}

// The control API uses roster-pinned mTLS once the node is enrolled; the
// CLI authenticates with the node's own cert (same host, same identity).
var (
	ctrlOnce   sync.Once
	ctrlHTTP   *http.Client
	ctrlScheme = "http"
	ctrlErr    error
)

func ctrlClientAndScheme() (*http.Client, string, error) {
	ctrlOnce.Do(func() {
		// The identity is root-only; without it the daemon rejects us, and plain HTTP
		// would only fail with a confusing "HTTP request to an HTTPS server".
		if err := p2pnode.CredsAccessError(); err != nil {
			ctrlErr = fmt.Errorf("cannot read the node's p2p identity (%v)\nThe p2p commands need root: run them with sudo", err)
			return
		}
		if conf, err := config.Read(); err == nil && conf.NodeUUID != "" {
			ctrlHTTP, ctrlScheme = p2pnode.NewMemberHTTPClient(conf.NodeUUID, nil, 90*time.Second)
			return
		}
		ctrlHTTP = &http.Client{Timeout: 90 * time.Second}
	})
	return ctrlHTTP, ctrlScheme, ctrlErr
}

// ctrlDo sends a request for path to the local control API.
func ctrlDo(method, path string, body io.Reader) (*http.Response, error) {
	client, scheme, err := ctrlClientAndScheme()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, fmt.Sprintf("%s://localhost:%d%s", scheme, controlPort, path), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, daemonHint(err)
	}
	return resp, nil
}

func ctrlPost(path string, body []byte, out interface{}) error {
	resp, err := ctrlDo(http.MethodPost, path, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeCtrl(resp, out)
}

func ctrlGet(path string, out interface{}) error {
	resp, err := ctrlDo(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeCtrl(resp, out)
}

func ctrlDelete(path string) error {
	resp, err := ctrlDo(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeCtrl(resp, nil)
}

func decodeCtrl(resp *http.Response, out interface{}) error {
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("control API error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func daemonHint(err error) error {
	return fmt.Errorf("%v\nIs the NodeEngine daemon running? Start it with: NodeEngine --p2p -d", err)
}

// nmGet queries the local NetManager control endpoints over its unix socket.
func nmGet(path string, out interface{}) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "/etc/netmanager/netmanager.sock")
			},
		},
	}
	resp, err := client.Get("http://localhost" + path)
	if err != nil {
		return fmt.Errorf("%v\nIs the NetManager running in p2p mode?", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeCtrl(resp, out)
}
