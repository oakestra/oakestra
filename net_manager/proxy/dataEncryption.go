package proxy

import (
	"NetManager/logger"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"sync/atomic"

	"golang.org/x/crypto/chacha20poly1305"
)

// Data-plane encryption for the tunnel. In p2p mode every payload sent over the
// tunnel socket is sealed with XChaCha20-Poly1305 under a key derived from the
// network secret, and anything that fails to open on arrival is dropped. Packets
// that stay on this node (Datapath.forwardResult's local shortcut) never touch the
// socket and are not encrypted. Cluster mode leaves encryption off.

// DeriveDataKey derives the 32-byte data-plane key from the shared network secret.
func DeriveDataKey(secret string) []byte {
	sum := sha256.Sum256([]byte("oakestra-data-plane|" + secret))
	return sum[:]
}

// dataCrypt seals and opens tunnel payloads. The zero value is disabled. The
// cipher sits behind an atomic pointer because a revocation rotates the key
// while the read loops are running.
type dataCrypt struct {
	aead atomic.Pointer[cipher.AEAD]
}

// sealScratch holds the buffers one batch is sealed into, reused across batches.
type sealScratch struct {
	nonces []byte
	bufs   [][]byte
	out    [][]byte
}

// EnableDataEncryption turns on (or rotates) payload encryption with a 32-byte key.
func (t *Tunnel) EnableDataEncryption(key []byte) error {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}
	t.crypt.aead.Store(&aead)
	logger.InfoLogger().Println("Data-plane encryption enabled (XChaCha20-Poly1305) 🔒")
	return nil
}

// seal returns bufs encrypted into scratch-owned buffers, each prefixed with
// its random nonce, or bufs unchanged when encryption is off. The nonces for a
// whole batch come from one read of the system RNG. Returns nil, dropping the
// batch, if the RNG fails.
func (c *dataCrypt) seal(bufs [][]byte, s *sealScratch) [][]byte {
	sealer := c.aead.Load()
	if sealer == nil {
		return bufs
	}
	aead := *sealer
	nonceSize := aead.NonceSize()
	need := nonceSize * len(bufs)
	if cap(s.nonces) < need {
		s.nonces = make([]byte, need)
	}
	nonces := s.nonces[:need]
	if _, err := rand.Read(nonces); err != nil {
		logger.ErrorLogger().Println("data-plane nonce generation failed, dropping batch:", err)
		return nil
	}
	for len(s.bufs) < len(bufs) {
		s.bufs = append(s.bufs, nil)
	}
	s.out = s.out[:0]
	for i, plain := range bufs {
		nonce := nonces[i*nonceSize : (i+1)*nonceSize]
		sealed := aead.Seal(append(s.bufs[i][:0], nonce...), nonce, plain, nil)
		s.bufs[i] = sealed
		s.out = append(s.out, sealed)
	}
	return s.out
}

// open decrypts a received datagram in place and returns the plaintext, moved to
// the start of buf where the TUN envelope expects it. ok is false for a datagram
// that is truncated, tampered with or sealed under another key. With encryption
// off, buf is returned unchanged.
func (c *dataCrypt) open(buf []byte) (plain []byte, ok bool) {
	opener := c.aead.Load()
	if opener == nil {
		return buf, true
	}
	aead := *opener
	nonceSize := aead.NonceSize()
	if len(buf) < nonceSize+aead.Overhead() {
		return nil, false
	}
	plain, err := aead.Open(buf[nonceSize:nonceSize], buf[:nonceSize], buf[nonceSize:], nil)
	if err != nil {
		return nil, false
	}
	return buf[:copy(buf, plain)], true
}
