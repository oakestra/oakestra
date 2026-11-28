// Package workercert holds the worker-side helpers for certificate bootstrap and renewal.
package workercert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GenerateKeyAndCSR creates a new private key and a CSR for it
func GenerateKeyAndCSR(commonName string, dnsNames []string, ips []net.IP) (keyPEM, csrPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:     pkix.Name{CommonName: commonName},
		DNSNames:    dnsNames,
		IPAddresses: ips,
	}, key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	return keyPEM, csrPEM, nil
}

// Load parses the first certificate of a PEM file
func Load(certFile string) (*x509.Certificate, error) {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("no certificate in %s", certFile)
	}
	return x509.ParseCertificate(block.Bytes)
}

// Info is what the worker reports about its certificate with every heartbeat.
type Info struct {
	Serial      string `json:"serial"`
	NotAfter    int64  `json:"not_after"`
	IssuerKeyId string `json:"issuer_key_id"`
}

var (
	infoMu      sync.Mutex
	infoCache   *Info
	infoFile    string
	infoModTime time.Time
)

// GetInfo returns the certificate's serial, expiry and issuing CA key ID.
// Result is cached until the file changes.
func GetInfo(certFile string) (*Info, error) {
	stat, err := os.Stat(certFile)
	if err != nil {
		return nil, err
	}
	infoMu.Lock()
	defer infoMu.Unlock()
	if infoCache != nil && infoFile == certFile && stat.ModTime().Equal(infoModTime) {
		return infoCache, nil
	}
	cert, err := Load(certFile)
	if err != nil {
		return nil, err
	}
	infoCache = &Info{
		Serial:      cert.SerialNumber.Text(16),
		NotAfter:    cert.NotAfter.Unix(),
		IssuerKeyId: hex.EncodeToString(cert.AuthorityKeyId),
	}
	infoFile = certFile
	infoModTime = stat.ModTime()
	return infoCache, nil
}

// WriteFileAtomic replaces path with data, so a crash never leaves a partial file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
