// Package airlock is the Go port of @devarno/sdk-js airlock/handoff-client.ts.
//
// Cross-apex auth flow F9: an Ed25519 handoff JWT bridges the apex boundary,
// after which each consuming app mints its own apex-local HS256 session JWT
// (audience "cross-apex-local-session"). Sparki backends only verify the
// local cookie — the handoff is the web-app's job. Keep this package in sync
// with the canonical TS source; the contract test in handoff_test.go pins
// byte-for-byte compatibility against a token signed by jose.
package airlock

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// LocalAlg is the JWT signing algorithm for the apex-local session cookie.
	LocalAlg = "HS256"
	// LocalAudience is the fixed audience claim on the apex-local session
	// cookie. Hard-coded by canonical contract.
	LocalAudience = "cross-apex-local-session"
)

// Session matches the TS CrossApexSession shape.
type Session struct {
	UserID    string
	Email     string
	Name      string
	Role      string
	ExpiresAt time.Time
}

// ErrInvalidSession means the token was missing, malformed, signed with the
// wrong secret, expired, has the wrong issuer/audience, or is otherwise
// untrusted. Callers should treat all subcases the same — emit an
// auth.decision deny span and redirect to handoff.
var ErrInvalidSession = errors.New("airlock: invalid session")

// VerifyLocalSession verifies a HS256 cookie value against the per-app secret.
// Returns nil + ErrInvalidSession on any failure (missing token, expired,
// tampered, wrong issuer or audience). issuer must be the consuming apex
// (e.g. "https://sparki.tools"), matching the value passed to signLocalSession
// when the cookie was minted.
func VerifyLocalSession(token, issuer, secret string) (*Session, error) {
	if token == "" || secret == "" {
		return nil, ErrInvalidSession
	}

	parsed, err := jwt.Parse(
		token,
		func(t *jwt.Token) (interface{}, error) {
			if t.Method.Alg() != LocalAlg {
				return nil, fmt.Errorf("unexpected alg %q", t.Method.Alg())
			}
			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{LocalAlg}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(LocalAudience),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidSession
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidSession
	}

	sub, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	if sub == "" || email == "" {
		return nil, ErrInvalidSession
	}
	name, _ := claims["name"].(string)
	role, _ := claims["role"].(string)
	if role == "" {
		role = "member"
	}
	expF, _ := claims["exp"].(float64)

	return &Session{
		UserID:    sub,
		Email:     email,
		Name:      name,
		Role:      role,
		ExpiresAt: time.Unix(int64(expF), 0),
	}, nil
}

// SignLocalSession is a Go-side signer for tests and any future Go code that
// needs to mint a local session (web-app remains the production signer).
// Mirrors the canonical TS signLocalSession.
func SignLocalSession(s Session, issuer, secret string, ttl time.Duration) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("airlock: session secret must be at least 32 characters")
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   s.UserID,
		"email": s.Email,
		"name":  s.Name,
		"role":  s.Role,
		"iss":   issuer,
		"aud":   LocalAudience,
		"iat":   now.Unix(),
		"exp":   now.Add(ttl).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(secret))
}
