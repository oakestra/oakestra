package requests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/workercert"
	"net/http"
)

// RenewWorkerCert replaces the worker certificate through the cluster gateway,
// authenticating with the current certificate
func RenewWorkerCert(cfg config.ConfFile) error {
	if cfg.WorkerCertFile == "" || cfg.WorkerKeyFile == "" {
		return fmt.Errorf("no worker certificate configured")
	}
	current, err := workercert.Load(cfg.WorkerCertFile)
	if err != nil {
		return fmt.Errorf("reading the current worker certificate failed: %v", err)
	}
	keyPEM, csrPEM, err := workercert.GenerateKeyAndCSR(
		current.Subject.CommonName, current.DNSNames, current.IPAddresses,
	)
	if err != nil {
		return fmt.Errorf("generating the new worker key failed: %v", err)
	}
	body, err := json.Marshal(map[string]string{"csr": string(csrPEM)})
	if err != nil {
		return err
	}

	client, proto, err := clusterClient(cfg)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s://%s:%d/api/certs/worker-renew", proto, cfg.ClusterAddress, cfg.ClusterPort)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("worker certificate renewal request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("worker certificate renewal failed with status %d", resp.StatusCode)
	}
	issued, err := readIssuedCert(resp.Body)
	if err != nil {
		return err
	}
	return installCert(cfg.WorkerKeyFile, keyPEM, cfg.WorkerCertFile, cfg.ClusterCaFile, issued)
}
