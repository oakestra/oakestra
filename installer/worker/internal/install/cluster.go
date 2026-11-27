package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/oakestra/oakestra/installer/util/command"
	"github.com/oakestra/oakestra/installer/util/download"
	"github.com/oakestra/oakestra/installer/util/prompt"
)

const (
	clusterProbeTimeout = 10 * time.Second
	defaultClusterPort  = 10100
	defaultRootPort     = 10000
)

type activeCluster struct {
	ID   string `json:"_id"`
	Name string `json:"cluster_name"`
	Host string `json:"ip"`
}

type clusterReachability uint8

const (
	clusterUnreachable clusterReachability = iota
	clusterReachable
	clusterReachableViaRoot
)

func (r clusterReachability) label() string {
	switch r {
	case clusterUnreachable:
		return lipgloss.NewStyle().Foreground(lipgloss.Red).Render("❌ unreachable")
	case clusterReachable:
		return lipgloss.NewStyle().Foreground(lipgloss.Green).Render("✅ reachable")
	case clusterReachableViaRoot:
		return lipgloss.NewStyle().Foreground(lipgloss.Yellow).Render("⚠️ reachable via root")
	default:
		return "unknown"
	}
}

type clusterProbe struct {
	name    string
	address clusterAddress
	status  clusterReachability
}

type clusterAddress struct {
	host string
	port int
	tls  bool
}

func (a clusterAddress) baseURL() url.URL {
	scheme := "http"
	if a.tls {
		scheme = "https"
	}
	return url.URL{Scheme: scheme, Host: net.JoinHostPort(a.host, strconv.Itoa(a.port))}
}

func (a clusterAddress) String() string {
	addressURL := a.baseURL()
	return addressURL.String()
}

func parseOrchestratorURL(address string, defaultPort int) (*url.URL, error) {
	address = strings.TrimSpace(address)
	needsScheme := !strings.Contains(address, "://")
	if needsScheme {
		host := strings.Trim(address, "[]")
		if net.ParseIP(host) != nil && strings.Contains(host, ":") {
			address = "[" + host + "]"
		}
		address = "http://" + address
	}
	addressURL, err := url.Parse(address)
	if err != nil || (addressURL.Scheme != "http" && addressURL.Scheme != "https") || addressURL.Hostname() == "" ||
		addressURL.User != nil || addressURL.ForceQuery || addressURL.RawQuery != "" || addressURL.Fragment != "" ||
		(strings.Count(addressURL.Host, ":") > 1 && !strings.HasPrefix(addressURL.Host, "[")) {
		return nil, errors.New("enter a hostname, IP address, or HTTP(S) URL")
	}
	if needsScheme && addressURL.Port() == "" {
		addressURL.Host = net.JoinHostPort(addressURL.Hostname(), strconv.Itoa(defaultPort))
	}
	return addressURL, nil
}

func parseClusterAddress(value string) (clusterAddress, error) {
	addressURL, err := parseOrchestratorURL(value, defaultClusterPort)
	if err != nil {
		return clusterAddress{}, fmt.Errorf("invalid Cluster Orchestrator address: %w", err)
	}
	if addressURL.Path != "" && addressURL.Path != "/" {
		return clusterAddress{}, errors.New("the Cluster Orchestrator URL must not include a path")
	}
	host := addressURL.Hostname()
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return clusterAddress{}, errors.New("invalid IPv6 address for the Cluster Orchestrator")
	}
	address := clusterAddress{host: host, port: defaultClusterPort, tls: addressURL.Scheme == "https"}
	if port := addressURL.Port(); port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return clusterAddress{}, errors.New("invalid Cluster Orchestrator port")
		}
		address.port = int(value)
	}
	return address, nil
}

func (i installer) selectCluster(ctx context.Context) (clusterProbe, error) {
	direct := true
	form := huh.NewForm(huh.NewGroup(huh.NewSelect[bool]().
		Title("How should the worker find its cluster?").
		Options(
			huh.NewOption("Enter a Cluster Orchestrator address", true),
			huh.NewOption("Discover clusters via a Root Orchestrator", false),
		).Value(&direct)))
	if err := prompt.RunForm(ctx, form); err != nil {
		return clusterProbe{}, err
	}
	if direct {
		return promptDirectCluster(ctx)
	}
	return i.discoverClusterViaRoot(ctx)
}

func promptDirectCluster(ctx context.Context) (clusterProbe, error) {
	var value string
	for {
		form := huh.NewForm(huh.NewGroup(huh.NewInput().
			Title("Cluster Orchestrator address").
			Placeholder("cluster.example.org").
			Value(&value).
			Validate(func(value string) error {
				_, err := parseClusterAddress(value)
				return err
			})))
		if err := prompt.RunForm(ctx, form); err != nil {
			return clusterProbe{}, err
		}
		address, err := parseClusterAddress(value)
		if err != nil {
			return clusterProbe{}, err
		}
		probe, err := probeDirectCluster(ctx, address)
		if err != nil || probe.status == clusterReachable {
			return probe, err
		}

		retry := true
		form = huh.NewForm(huh.NewGroup(huh.NewSelect[bool]().
			Title(fmt.Sprintf("Cluster at %s is unreachable", address)).
			Options(
				huh.NewOption("Enter another address", true),
				huh.NewOption("Continue anyway", false),
			).Value(&retry)))
		if err := prompt.RunForm(ctx, form); err != nil {
			return clusterProbe{}, err
		}
		if !retry {
			return probe, nil
		}
	}
}

func promptRootAddress(ctx context.Context) (*url.URL, error) {
	localRoot := &url.URL{Scheme: "http", Host: net.JoinHostPort("localhost", strconv.Itoa(defaultRootPort))}
	localCtx, cancel := context.WithTimeout(ctx, clusterProbeTimeout)
	_, err := listActiveClusters(localCtx, localRoot)
	cancel()

	var address string
	if err == nil {
		address = localRoot.String()
	}
	form := huh.NewForm(huh.NewGroup(huh.NewInput().
		Title("Root Orchestrator address").
		Placeholder("root.example.org").
		Value(&address).
		Validate(func(value string) error {
			_, err := parseOrchestratorURL(value, defaultRootPort)
			return err
		})))
	if err := prompt.RunForm(ctx, form); err != nil {
		return nil, err
	}
	return parseOrchestratorURL(address, defaultRootPort)
}

func (i installer) discoverClusterViaRoot(ctx context.Context) (clusterProbe, error) {
	rootURL, err := promptRootAddress(ctx)
	if err != nil {
		return clusterProbe{}, err
	}
	clusters, err := listActiveClusters(ctx, rootURL)
	if err != nil {
		return clusterProbe{}, err
	}
	if len(clusters) == 0 {
		return clusterProbe{}, errors.New("no active clusters found")
	}
	i.logger.InfoContext(ctx, "Probing cluster reachability")
	probes, err := probeClusters(ctx, clusters, rootURL.Hostname())
	if err != nil {
		return clusterProbe{}, err
	}
	selected := probes[0]
	if len(probes) > 1 {
		options := make([]huh.Option[clusterProbe], 0, len(probes))
		for _, probe := range probes {
			label := fmt.Sprintf("%s (%s) · %s", probe.name, probe.address.host, probe.status.label())
			options = append(options, huh.NewOption(label, probe))
		}
		form := huh.NewForm(huh.NewGroup(huh.NewSelect[clusterProbe]().
			Title("Select a cluster").Options(options...).Value(&selected)))
		if err := prompt.RunForm(ctx, form); err != nil {
			return clusterProbe{}, err
		}
	}
	if selected.address.host == "" {
		return clusterProbe{}, fmt.Errorf("cluster %q has no address", selected.name)
	}
	return selected, nil
}

func (i installer) configureCluster(ctx context.Context, selected clusterProbe) error {
	logger := i.logger.With("address", selected.address.String())
	if selected.name != "" {
		logger = logger.With("cluster", selected.name)
	}
	if selected.status == clusterUnreachable {
		logger.WarnContext(ctx, "Selected cluster is currently unreachable")
	}
	logger.InfoContext(ctx, "Configuring NodeEngine cluster")

	args := []string{"config", "cluster"}
	if selected.address.port != defaultClusterPort {
		args = append(args, "--clusterPort", strconv.Itoa(selected.address.port))
	}
	if selected.address.tls {
		args = append(args, "--clusterSSL")
	}
	host := selected.address.host
	if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		// NodeEngine inserts the configured address directly into its URLs.
		host = "[" + host + "]"
	}
	return command.RunSilent(ctx, "/bin/NodeEngine", append(args, "--", host)...)
}

func listActiveClusters(ctx context.Context, rootURL *url.URL) ([]activeCluster, error) {
	data, err := download.GetBytes(ctx, rootURL.JoinPath("api/clusters/active").String())
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return nil, fmt.Errorf("list active clusters: %w", err)
	}
	var clusters []activeCluster
	if err := json.Unmarshal(data, &clusters); err != nil {
		return nil, fmt.Errorf("decode active clusters: %w", err)
	}
	return clusters, nil
}

func probeClusters(ctx context.Context, clusters []activeCluster, rootHost string) ([]clusterProbe, error) {
	probes := make([]clusterProbe, len(clusters))
	root := clusterAddress{host: rootHost, port: defaultClusterPort}
	var wg sync.WaitGroup
	for index, cluster := range clusters {
		wg.Go(func() {
			direct := clusterAddress{host: strings.Trim(cluster.Host, "[]"), port: defaultClusterPort}
			probe := clusterProbe{name: cluster.Name, address: direct, status: clusterUnreachable}
			switch {
			case probeCluster(ctx, cluster.ID, direct):
				probe.status = clusterReachable
			case root != direct && probeCluster(ctx, cluster.ID, root):
				probe.address = root
				probe.status = clusterReachableViaRoot
			}
			probes[index] = probe
		})
	}
	wg.Wait()
	return probes, ctx.Err()
}

func probeCluster(ctx context.Context, id string, address clusterAddress) bool {
	if id == "" || address.host == "" {
		return false
	}
	status, err := readClusterStatus(ctx, address)
	return err == nil && status.ID == id
}

func probeDirectCluster(ctx context.Context, address clusterAddress) (clusterProbe, error) {
	status, err := readClusterStatus(ctx, address)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return clusterProbe{}, ctxErr
	}
	if err != nil {
		return clusterProbe{address: address, status: clusterUnreachable}, nil
	}
	return clusterProbe{name: status.Name, address: address, status: clusterReachable}, nil
}

type clusterStatus struct {
	ID              string `json:"cluster_id"`
	Name            string `json:"cluster_name"`
	ConnectedToRoot *bool  `json:"connected_to_root"`
}

func readClusterStatus(ctx context.Context, address clusterAddress) (clusterStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, clusterProbeTimeout)
	defer cancel()

	statusURL := address.baseURL()
	statusURL.Path = "/api/cluster/status"
	data, err := download.GetBytes(ctx, statusURL.String())
	if err != nil {
		return clusterStatus{}, err
	}
	var status clusterStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return clusterStatus{}, err
	}
	if status.ID == "" && status.ConnectedToRoot == nil {
		return clusterStatus{}, errors.New("missing cluster status fields")
	}
	return status, nil
}
