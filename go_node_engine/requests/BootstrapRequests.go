package requests

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/logger"
	"io"
	"net/http"
	"os"
	"path"
	"time"
)

// CERT_DIR is where the bootstrap stores the worker's certificate material.
var CERT_DIR = path.Join("/etc", "oakestra", "certs")

type workerBootstrapResponse struct {
	PrivateKey  string `json:"private_key"`
	Certificate string `json:"certificate"`
	RootCa      string `json:"root_ca"`
}

// WorkerBootstrap redeems a one-time registration token for worker
// certificates. No-op when no token is configured or certificates are
// already present. On success the updated configuration (cert paths set,
// token cleared) is persisted and returned.
func WorkerBootstrap(cfg config.ConfFile) (config.ConfFile, error) {
	if cfg.ClusterToken == "" {
		return cfg, nil
	}
	if certFilesPresent(cfg) {
		logger.InfoLogger().Printf("Recreating worker certificates.")
	}

	baseURL := fmt.Sprintf("https://%s:%d", cfg.ClusterAddress, cfg.ClusterPort)
	logger.InfoLogger().Printf("Bootstrapping worker certificates from %s", baseURL)

	caPath, client, err := fetchClusterCA(cfg, baseURL)
	if err != nil {
		return cfg, fmt.Errorf("fetching cluster CA failed: %v", err)
	}

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "oakestra-worker"
	}
	body, err := json.Marshal(map[string]interface{}{
		"token":       cfg.ClusterToken,
		"common_name": hostname,
		"alt_names":   []string{hostname, "localhost"},
	})
	if err != nil {
		return cfg, err
	}

	resp, err := client.Post(baseURL+"/api/certs/worker-bootstrap", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return cfg, fmt.Errorf("worker bootstrap request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusUnauthorized {
		return cfg, fmt.Errorf("registration token rejected (invalid, expired, or already used) — mint a fresh token and retry")
	}
	if resp.StatusCode != http.StatusOK {
		return cfg, fmt.Errorf("worker bootstrap failed with status %d", resp.StatusCode)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return cfg, err
	}
	bootstrap := workerBootstrapResponse{}
	if err := json.Unmarshal(respBytes, &bootstrap); err != nil {
		return cfg, fmt.Errorf("invalid worker bootstrap response: %v", err)
	}
	if bootstrap.PrivateKey == "" || bootstrap.Certificate == "" || bootstrap.RootCa == "" {
		return cfg, fmt.Errorf("incomplete worker bootstrap response")
	}

	certPath := path.Join(CERT_DIR, "worker.crt")
	keyPath := path.Join(CERT_DIR, "worker.key")
	if err := os.MkdirAll(CERT_DIR, 0755); err != nil {
		return cfg, err
	}
	if err := os.WriteFile(certPath, []byte(bootstrap.Certificate), 0644); err != nil {
		return cfg, err
	}
	if err := os.WriteFile(keyPath, []byte(bootstrap.PrivateKey), 0600); err != nil {
		return cfg, err
	}
	// The root CA is the trust anchor for both the cluster gateway (fallback
	// cert) and the MQTT broker.
	if err := os.WriteFile(caPath, []byte(bootstrap.RootCa), 0644); err != nil {
		return cfg, err
	}

	cfg.WorkerCertFile = certPath
	cfg.WorkerKeyFile = keyPath
	cfg.ClusterCaFile = caPath
	cfg.ClusterSSL = true
	logger.InfoLogger().Printf("Worker certificates installed in %s", CERT_DIR)
	return clearToken(cfg)
}

func certFilesPresent(cfg config.ConfFile) bool {
	if cfg.WorkerCertFile == "" || cfg.WorkerKeyFile == "" {
		return false
	}
	for _, file := range []string{cfg.WorkerCertFile, cfg.WorkerKeyFile} {
		if _, err := os.Stat(file); err != nil {
			return false
		}
	}
	return true
}

func clearToken(cfg config.ConfFile) (config.ConfFile, error) {
	cfg.ClusterToken = ""
	if err := config.GetConfFileManager().Write(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// fetchClusterCA establishes trust for the bootstrap call. Returns the path
// of the stored CA and an HTTP client that verifies the gateway accordingly.
func fetchClusterCA(cfg config.ConfFile, baseURL string) (string, *http.Client, error) {
	caPath := path.Join(CERT_DIR, "ca.crt")

	if cfg.ClusterGatewayTrust == "system" {
		// Gateway presents a publicly trusted certificate; also fetch the
		// internal root CA over the verified channel for MQTT trust later.
		client := &http.Client{Timeout: 30 * time.Second}
		if err := downloadCA(client, baseURL, caPath); err != nil {
			return "", nil, err
		}
		return caPath, client, nil
	}

	if cfg.ClusterGatewayTrust != "" && cfg.ClusterGatewayTrust != "insecure" {
		// Custom CA bundle for the gateway's (privately issued) server cert.
		bundle, err := os.ReadFile(cfg.ClusterGatewayTrust)
		if err != nil {
			return "", nil, fmt.Errorf("reading gateway trust bundle %s: %v", cfg.ClusterGatewayTrust, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(bundle) {
			return "", nil, fmt.Errorf("failed to parse gateway trust bundle %s", cfg.ClusterGatewayTrust)
		}
		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			}},
			Timeout: 30 * time.Second,
		}
		if err := downloadCA(client, baseURL, caPath); err != nil {
			return "", nil, err
		}
		return caPath, client, nil
	}

	// TOFU (cfg.ClusterGatewayTrust == "" or "insecure"): no trust anchor yet —
	// fetch the internal root CA over an unverified connection and pin it for
	// the redemption call that follows.
	logger.InfoLogger().Printf("No trust anchor configured — fetching cluster CA without server verification (TOFU).")
	insecureClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 -- intentional TOFU bootstrap
			MinVersion:         tls.VersionTLS12,
		}},
		Timeout: 30 * time.Second,
	}
	if err := downloadCA(insecureClient, baseURL, caPath); err != nil {
		return "", nil, err
	}

	caPem, err := os.ReadFile(caPath)
	if err != nil {
		return "", nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPem) {
		return "", nil, fmt.Errorf("failed to parse cluster CA PEM")
	}
	pinnedClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}},
		Timeout: 30 * time.Second,
	}
	return caPath, pinnedClient, nil
}

func downloadCA(client *http.Client, baseURL string, caPath string) error {
	resp, err := client.Get(baseURL + "/api/certs/ca.crt")
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CA download failed with status %d", resp.StatusCode)
	}
	caPem, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(CERT_DIR, 0755); err != nil {
		return err
	}
	return os.WriteFile(caPath, caPem, 0644)
}
