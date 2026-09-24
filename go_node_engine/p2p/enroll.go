package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/logger"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Security & onboarding (plan §9, trust model D — no CA):
//   - every node has a locally-generated keypair + SELF-SIGNED cert (§9.4)
//   - trust = a roster of member cert fingerprints, handed over at enrollment
//   - admission = single-use short-TTL join tokens minted by any member (§9.2, §9.6)
//   - the enrollment endpoint (:50106, TLS) is run by EVERY member, so any member
//     can onboard and only members can (they alone hold the PSK + token mint) (§9.3)

// credsDirPath is a var (not const) so tests can point it at a temp dir.
var credsDirPath = "/etc/oakestra/p2p"

// Creds is the persisted p2p identity + trust state (plan §9; /etc/oakestra/p2p/).
type Creds struct {
	GossipKey string            `json:"gossip_key"`        // PSK passphrase (keyed via DeriveGossipKey)
	Roster    map[string]string `json:"roster"`            // node UUID -> cert fingerprint (sha256:...)
	Seeds     []string          `json:"seeds,omitempty"`   // known gossip endpoints
}

type issuedToken struct {
	ID     string `json:"id"`
	Hash   string `json:"hash"` // sha256 of the full token — the secret itself is never stored
	Expiry int64  `json:"expiry"`
	Used   bool   `json:"used"`
}

func credsDir() (string, error) {
	if err := os.MkdirAll(credsDirPath, 0o700); err != nil {
		return "", err
	}
	return credsDirPath, nil
}

// LoadCreds returns the stored credentials, or ok=false if the node is not enrolled.
func LoadCreds() (Creds, bool) {
	var c Creds
	data, err := os.ReadFile(filepath.Join(credsDirPath, "creds.json"))
	if err != nil || json.Unmarshal(data, &c) != nil || c.GossipKey == "" {
		return Creds{}, false
	}
	return c, true
}

func SaveCreds(c Creds) error {
	dir, err := credsDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "creds.json"), data, 0o600)
}

// EnsureIdentity loads or creates the node's keypair + self-signed cert (plan §9.4:
// the private key never leaves the node). Returns the TLS certificate and the
// cert fingerprint ("sha256:<hex>").
func EnsureIdentity(nodeUUID string) (tls.Certificate, string, error) {
	dir, err := credsDir()
	if err != nil {
		return tls.Certificate{}, "", err
	}
	certPath := filepath.Join(dir, "node.crt")
	keyPath := filepath.Join(dir, "node.key")

	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		return cert, Fingerprint(cert.Certificate[0]), nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: nodeUUID, Organization: []string{"oakestra-p2p"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(5 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	keyDer, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	certPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer})
	if err := os.WriteFile(certPath, certPem, 0o600); err != nil {
		return tls.Certificate{}, "", err
	}
	if err := os.WriteFile(keyPath, keyPem, 0o600); err != nil {
		return tls.Certificate{}, "", err
	}
	cert, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	return cert, Fingerprint(der), nil
}

// Fingerprint returns "sha256:<hex>" of a DER certificate — the roster identity (§9.4).
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Genesis creates a brand-new p2p network on this node (plan §1.2 `--init`, §9.4):
// mints the gossip PSK, generates the self-signed identity, rosters self.
func Genesis(nodeUUID string) (Creds, error) {
	if creds, ok := LoadCreds(); ok {
		return creds, nil // already initialised — idempotent
	}
	_, fp, err := EnsureIdentity(nodeUUID)
	if err != nil {
		return Creds{}, err
	}
	pskBytes := make([]byte, 32)
	if _, err := rand.Read(pskBytes); err != nil {
		return Creds{}, err
	}
	creds := Creds{
		GossipKey: hex.EncodeToString(pskBytes),
		Roster:    map[string]string{nodeUUID: fp},
	}
	if err := SaveCreds(creds); err != nil {
		return Creds{}, err
	}
	logger.InfoLogger().Printf("p2p genesis: new network created (node %s rostered)", nodeUUID)
	return creds, nil
}

// ---------- join tokens (plan §9.2, §9.6) ----------

func tokensPath() string { return filepath.Join(credsDirPath, "tokens.json") }

func loadTokens() []issuedToken {
	var list []issuedToken
	if data, err := os.ReadFile(tokensPath()); err == nil {
		_ = json.Unmarshal(data, &list)
	}
	return list
}

func saveTokens(list []issuedToken) error {
	dir, err := credsDir()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(list)
	return os.WriteFile(filepath.Join(dir, "tokens.json"), data, 0o600)
}

// MintToken creates a single-use join token "<id>.<secret>" valid for ttl. Only the
// token's hash is stored (plan §9.6).
func MintToken(ttl time.Duration) (string, error) {
	idB := make([]byte, 3)
	secretB := make([]byte, 12)
	if _, err := rand.Read(idB); err != nil {
		return "", err
	}
	if _, err := rand.Read(secretB); err != nil {
		return "", err
	}
	token := hex.EncodeToString(idB) + "." + hex.EncodeToString(secretB)
	sum := sha256.Sum256([]byte(token))
	list := loadTokens()
	list = append(list, issuedToken{
		ID:     hex.EncodeToString(idB),
		Hash:   hex.EncodeToString(sum[:]),
		Expiry: time.Now().Add(ttl).Unix(),
	})
	return token, saveTokens(list)
}

// consumeToken validates a presented token and marks it used (single-use, plan §9.6).
func consumeToken(token string) error {
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	list := loadTokens()
	for i, t := range list {
		if t.Hash != hash {
			continue
		}
		if t.Used {
			return errors.New("token already used")
		}
		if time.Now().Unix() > t.Expiry {
			return errors.New("token expired")
		}
		list[i].Used = true
		return saveTokens(list)
	}
	return errors.New("unknown token")
}

// ---------- enrollment endpoint + join flow (plan §9.3) ----------

type enrollRequest struct {
	Token       string `json:"token"`
	NodeUUID    string `json:"node_uuid"`
	Fingerprint string `json:"fingerprint"`
	GossipPort  int    `json:"gossip_port"`
}

type enrollResponse struct {
	GossipKey string            `json:"gossip_key"`
	Roster    map[string]string `json:"roster"`
	Seeds     []string          `json:"seeds"`
}

// ServeEnrollment runs the members-only onboarding endpoint on :<enrollPort> over TLS
// with the node's self-signed cert. Every member runs it — "any node can onboard" (§9.3).
func ServeEnrollment(nodeUUID string, enrollPort, gossipPort int) error {
	cert, _, err := EnsureIdentity(nodeUUID)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/enroll", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req enrollRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := consumeToken(req.Token); err != nil {
			logger.ErrorLogger().Printf("p2p enroll: rejected join from %s: %v", req.NodeUUID, err)
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		creds, ok := LoadCreds()
		if !ok {
			http.Error(w, "this node is not an enrolled member", http.StatusServiceUnavailable)
			return
		}
		// Admit: roster the new node's fingerprint (plan §9.4 — a PSK-authenticated
		// roster update; gossiped to the mesh via the registry on next advertisement).
		creds.Roster[req.NodeUUID] = req.Fingerprint
		_ = SaveCreds(creds)

		seeds := []string{fmt.Sprintf("%s:%d", requestHostIP(r), gossipPort)}
		writeJSON(w, enrollResponse{GossipKey: creds.GossipKey, Roster: creds.Roster, Seeds: seeds})
		logger.InfoLogger().Printf("p2p enroll: admitted %s (%s)", req.NodeUUID, req.Fingerprint[:23])
	})

	server := &http.Server{
		Addr:      fmt.Sprintf(":%d", enrollPort),
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	logger.InfoLogger().Printf("p2p: enrollment endpoint listening on :%d (TLS)", enrollPort)
	return server.ListenAndServeTLS("", "")
}

// requestHostIP extracts the IP the joiner reached us on (best seed address for it).
func requestHostIP(r *http.Request) string {
	host := r.Host
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

// Join performs the enrollment handshake against a bootstrap member (plan §9.3):
// verify the trust-anchor pin, present token + our fingerprint, receive PSK + roster.
func Join(nodeUUID string, pending config.PendingEnroll) (Creds, error) {
	_, fp, err := EnsureIdentity(nodeUUID)
	if err != nil {
		return Creds{}, err
	}
	pin := strings.TrimPrefix(pending.CaHash, "sha256:")

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				// Self-signed certs: authenticity comes from the out-of-band pin
				// (--ca-hash) carried in the addp2p command (plan §9.2) — defeats MITM.
				InsecureSkipVerify: true, //nolint:gosec // pin-verified below
				VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
					if len(rawCerts) == 0 {
						return errors.New("no peer certificate")
					}
					sum := sha256.Sum256(rawCerts[0])
					if hex.EncodeToString(sum[:]) != pin {
						return errors.New("trust-anchor pin mismatch — possible MITM, refusing to join")
					}
					return nil
				},
			},
		},
	}

	body, _ := json.Marshal(enrollRequest{Token: pending.Token, NodeUUID: nodeUUID, Fingerprint: fp})
	resp, err := client.Post(fmt.Sprintf("https://%s/enroll", pending.Join), "application/json", bytes.NewBuffer(body))
	if err != nil {
		return Creds{}, fmt.Errorf("enrollment failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return Creds{}, fmt.Errorf("enrollment rejected: %s", strings.TrimSpace(string(msg)))
	}
	var er enrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return Creds{}, err
	}
	creds := Creds{GossipKey: er.GossipKey, Roster: er.Roster, Seeds: er.Seeds}
	if creds.Roster == nil {
		creds.Roster = map[string]string{}
	}
	creds.Roster[nodeUUID] = fp
	if err := SaveCreds(creds); err != nil {
		return Creds{}, err
	}
	logger.InfoLogger().Printf("p2p: enrolled via %s 🟢 (%d member(s) in roster)", pending.Join, len(creds.Roster))
	return creds, nil
}

// Revoke drops a node from the local roster (plan §9.6). NOTE: full lock-out also
// requires gossip-PSK + data-key rotation across the mesh, which is not yet automated —
// documented limitation; rotate by re-running genesis + re-enrolling members.
func Revoke(nodeUUID string) error {
	creds, ok := LoadCreds()
	if !ok {
		return errors.New("this node is not enrolled in a p2p network")
	}
	if _, present := creds.Roster[nodeUUID]; !present {
		return fmt.Errorf("node %s is not in the roster", nodeUUID)
	}
	delete(creds.Roster, nodeUUID)
	return SaveCreds(creds)
}
