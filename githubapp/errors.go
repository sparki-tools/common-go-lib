package githubapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors. Callers branch with errors.Is; *APIError carries detail.
var (
	// ErrInvalidKey reports a private key that cannot be parsed or is not RSA.
	ErrInvalidKey = errors.New("githubapp: invalid private key")
	// ErrNotInstalled reports a 404: the App is not installed on the account
	// or repository, the installation no longer exists, or it cannot see the repo.
	ErrNotInstalled = errors.New("githubapp: app not installed")
	// ErrUnauthorized reports a 401: GitHub rejected the app JWT or token,
	// typically a wrong key, wrong AppID or a clock far out of sync.
	ErrUnauthorized = errors.New("githubapp: unauthorized")
	// ErrRateLimited reports a 429, or a 403 carrying rate-limit headers.
	ErrRateLimited = errors.New("githubapp: rate limited")
)

const maxMessageLen = 200

// APIError is a non-2xx response from GitHub. It unwraps to ErrNotInstalled,
// ErrUnauthorized or ErrRateLimited when the status maps to one of them.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	// Message is GitHub's "message" field, truncated, with the request
	// credential scrubbed.
	Message string
	// RetryAfter is the Retry-After header value, when present.
	RetryAfter time.Duration
	// RateLimitReset is the X-RateLimit-Reset time, when present.
	RateLimitReset time.Time

	kind error
}

// Error implements error. It never includes credentials.
func (e *APIError) Error() string {
	var b strings.Builder

	fmt.Fprintf(&b, "githubapp: %s %s: status %d", e.Method, e.Path, e.StatusCode)

	if e.kind != nil {
		b.WriteString(" (" + strings.TrimPrefix(e.kind.Error(), "githubapp: ") + ")")
	}

	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}

	return b.String()
}

// Unwrap returns the matching sentinel error, or nil.
func (e *APIError) Unwrap() error { return e.kind }

func newAPIError(method, path string, resp *http.Response, body []byte, credential string, now time.Time) *APIError {
	e := &APIError{Method: method, Path: path, StatusCode: resp.StatusCode}

	var payload struct {
		Message string `json:"message"`
	}

	if json.Unmarshal(body, &payload) == nil {
		e.Message = sanitize(payload.Message, credential)
	}

	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
	}

	if s := resp.Header.Get("X-RateLimit-Reset"); s != "" {
		if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
			e.RateLimitReset = time.Unix(unix, 0)
			if e.RetryAfter == 0 && e.RateLimitReset.After(now) {
				e.RetryAfter = e.RateLimitReset.Sub(now)
			}
		}
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		e.kind = ErrNotInstalled
	case http.StatusUnauthorized:
		e.kind = ErrUnauthorized
	case http.StatusTooManyRequests:
		e.kind = ErrRateLimited
	case http.StatusForbidden:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "" {
			e.kind = ErrRateLimited
		}
	}

	return e
}

// sanitize truncates msg and removes the credential if GitHub (or anything
// in between) echoed it back.
func sanitize(msg, credential string) string {
	if credential != "" {
		msg = strings.ReplaceAll(msg, credential, "[REDACTED]")
	}

	if len(msg) > maxMessageLen {
		msg = msg[:maxMessageLen] + "..."
	}

	return msg
}
