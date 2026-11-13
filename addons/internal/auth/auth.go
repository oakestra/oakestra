// Package auth verifies the RS256 access tokens that system_manager hands out (signed by
// jwt_generator) and provides net/http middleware for user and admin endpoints.
//
// The token layout comes from flask_jwt_extended plus system_manager's additional claims:
// "sub" (username), "type" ("access" or "refresh"), "exp", "roles", "organization", "user" and,
// on file download tokens, a truthy "file_access_token".
//
// Unlike system_manager's require_role, this package doesn't look the user up in Mongo, so a
// deleted user or a revoked role stays valid until the token expires (10 minutes by default).
package auth

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AdminRole is the role system_manager gives platform admins (roles/securityUtils.py).
const AdminRole = "Admin"

// DefaultLeeway absorbs small clock skew between jwt_generator and this service.
const DefaultLeeway = 30 * time.Second

// Claims are the claims of a system_manager access token.
type Claims struct {
	jwt.RegisteredClaims
	// Type is "access" or "refresh" (flask_jwt_extended).
	Type         string   `json:"type"`
	Roles        []string `json:"roles"`
	Organization string   `json:"organization"`
	User         string   `json:"user"`
	// FileAccessToken is set on download tokens, which must not work as API tokens. It is
	// kept as `any` because system_manager checks Python truthiness, not a specific type.
	FileAccessToken any `json:"file_access_token,omitempty"`
}

// HasRole reports whether the token carries the role.
func (c *Claims) HasRole(role string) bool {
	return slices.Contains(c.Roles, role)
}

// IsAdmin reports whether the token carries AdminRole.
func (c *Claims) IsAdmin() bool {
	return c.HasRole(AdminRole)
}

// Verification errors. Verify wraps them, so use errors.Is.
var (
	ErrInvalidToken = errors.New("invalid token")
	ErrNotAccess    = errors.New("only access tokens are allowed")
	ErrNoKey        = errors.New("no verification key loaded")
)

// Verifier checks tokens against jwt_generator's public key. It is safe for concurrent use,
// and the key can be swapped at run time with SetKey (jwt_generator makes a new key pair on
// every restart unless one is configured).
type Verifier struct {
	key    atomic.Pointer[rsa.PublicKey]
	leeway time.Duration
	// now is replaced in tests.
	now func() time.Time
}

// NewVerifier returns a verifier using key and DefaultLeeway. key may be nil if it is set
// later with SetKey; until then every token is rejected.
func NewVerifier(key *rsa.PublicKey) *Verifier {
	v := &Verifier{leeway: DefaultLeeway, now: time.Now}
	if key != nil {
		v.key.Store(key)
	}
	return v
}

// SetKey replaces the verification key.
func (v *Verifier) SetKey(key *rsa.PublicKey) {
	v.key.Store(key)
}

// Verify parses and checks a raw token: RS256 signature with the current key, expiry
// (required), not-before, type "access", a subject, and no file access flag.
func (v *Verifier) Verify(raw string) (*Claims, error) {
	key := v.key.Load()
	if key == nil {
		return nil, ErrNoKey
	}
	parser := jwt.NewParser(
		// Pinning the method is what stops alg=none and HS256-signed-with-the-public-key.
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(v.leeway),
		jwt.WithTimeFunc(v.now),
	)
	var claims Claims
	_, err := parser.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) { return key, nil })
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.Type != "access" || truthy(claims.FileAccessToken) {
		return nil, ErrNotAccess
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: missing subject", ErrInvalidToken)
	}
	return &claims, nil
}

// truthy mirrors Python's truthiness for JSON values.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	default:
		return true
	}
}
