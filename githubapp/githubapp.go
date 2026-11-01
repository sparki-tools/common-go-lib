// Package githubapp authenticates as a GitHub App and mints installation
// access tokens.
//
// It signs the app JWT (RS256) itself, exchanges it for installation tokens
// and caches those tokens in memory until five minutes before they expire.
// It uses only the standard library: there is no third-party GitHub client.
//
// Credentials (the private key, app JWTs and installation tokens) never
// appear in error strings.
package githubapp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the GitHub REST API root used when Config.BaseURL is empty.
	DefaultBaseURL = "https://api.github.com"

	// jwtBackdate offsets iat into the past to absorb clock skew with GitHub.
	jwtBackdate = 60 * time.Second
	// jwtLifetime is the app JWT lifetime; GitHub's maximum is 10 minutes.
	jwtLifetime = 9 * time.Minute
	// tokenRefreshMargin is how long before expiry a cached token is replaced.
	tokenRefreshMargin = 5 * time.Minute

	apiVersion   = "2022-11-28"
	userAgent    = "sparki-githubapp"
	maxBodyBytes = 1 << 20
)

// Config configures a Client.
type Config struct {
	// AppID is the numeric GitHub App ID; it becomes the JWT iss claim.
	AppID int64
	// PrivateKeyPEM is the App's RSA private key, PKCS#1 or PKCS#8 PEM.
	PrivateKeyPEM []byte
	// BaseURL is the REST API root. Defaults to DefaultBaseURL.
	BaseURL string
	// HTTPClient sends requests. Defaults to http.DefaultClient.
	HTTPClient *http.Client
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
}

// Client mints app JWTs and installation tokens. It is safe for concurrent use.
type Client struct {
	appID   int64
	key     *rsa.PrivateKey
	baseURL string
	http    *http.Client
	now     func() time.Time

	mu       sync.Mutex
	cache    map[string]Token
	inflight map[string]*call
}

// New validates cfg and returns a Client. A missing AppID or an unparseable
// key fails here rather than on the first request.
func New(cfg Config) (*Client, error) {
	if cfg.AppID <= 0 {
		return nil, errors.New("githubapp: AppID must be positive")
	}

	key, err := parsePrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("githubapp: invalid BaseURL: %w", err)
	}

	c := &Client{
		appID:    cfg.AppID,
		key:      key,
		baseURL:  baseURL,
		http:     cfg.HTTPClient,
		now:      cfg.Now,
		cache:    make(map[string]Token),
		inflight: make(map[string]*call),
	}
	if c.http == nil {
		c.http = http.DefaultClient
	}

	if c.now == nil {
		c.now = time.Now
	}

	return c, nil
}

// parsePrivateKey accepts PKCS#1 ("RSA PRIVATE KEY") and PKCS#8 ("PRIVATE KEY")
// PEM. Errors wrap ErrInvalidKey and never quote the key material.
func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block found", ErrInvalidKey)
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: malformed PKCS#1 key", ErrInvalidKey)
		}

		return key, nil
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: malformed PKCS#8 key", ErrInvalidKey)
		}

		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%w: PKCS#8 key is not RSA", ErrInvalidKey)
		}

		return key, nil
	default:
		return nil, fmt.Errorf("%w: unsupported PEM block type %q", ErrInvalidKey, block.Type)
	}
}

// appJWT signs a fresh RS256 app JWT: iat = now-60s, exp = now+9m, iss = AppID.
func (c *Client) appJWT() (string, error) {
	now := c.now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]int64{
		"iat": now.Add(-jwtBackdate).Unix(),
		"exp": now.Add(jwtLifetime).Unix(),
		"iss": c.appID,
	}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("githubapp: encode jwt header: %w", err)
	}

	cb, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("githubapp: encode jwt claims: %w", err)
	}

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(hb) + "." + enc.EncodeToString(cb)
	digest := sha256.Sum256([]byte(signingInput))

	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", errors.New("githubapp: sign jwt failed")
	}

	return signingInput + "." + enc.EncodeToString(sig), nil
}

// do sends an authenticated request and decodes a 2xx JSON response into out.
// credential is the bearer value; it is scrubbed from any error text.
func (c *Client) do(ctx context.Context, method, path, credential string, body, out any) error {
	var reqBody io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("githubapp: encode request body: %w", err)
		}

		reqBody = strings.NewReader(string(b))
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("githubapp: build request %s %s: %w", method, path, err)
	}

	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Transport errors carry the URL, never headers, so they hold no credential.
		return fmt.Errorf("githubapp: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("githubapp: read response %s %s: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return newAPIError(method, path, resp, data, credential, c.now())
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("githubapp: decode response %s %s: %w", method, path, err)
	}

	return nil
}

// Installation resolves owner/repo to the App's installation ID (via
// GET /repos/{owner}/{repo}/installation with the app JWT) and the repository's
// numeric ID (via GET /repos/{owner}/{repo} with a metadata:read installation
// token). It returns an error wrapping ErrNotInstalled when the App is not
// installed on the repository or cannot see it.
func (c *Client) Installation(ctx context.Context, owner, repo string) (installationID, repoID int64, err error) {
	if owner == "" || repo == "" {
		return 0, 0, errors.New("githubapp: owner and repo are required")
	}

	repoPath := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)

	jwt, err := c.appJWT()
	if err != nil {
		return 0, 0, err
	}

	var inst struct {
		ID int64 `json:"id"`
	}

	err = c.do(ctx, http.MethodGet, repoPath+"/installation", jwt, nil, &inst)
	if err != nil {
		return 0, 0, err
	}

	if inst.ID == 0 {
		return 0, 0, errors.New("githubapp: installation response has no id")
	}

	tok, err := c.InstallationToken(ctx, inst.ID, TokenOptions{Permissions: map[string]string{"metadata": "read"}})
	if err != nil {
		return 0, 0, err
	}

	var r struct {
		ID int64 `json:"id"`
	}

	err = c.do(ctx, http.MethodGet, repoPath, tok.Value, nil, &r)
	if err != nil {
		return 0, 0, err
	}

	if r.ID == 0 {
		return 0, 0, errors.New("githubapp: repository response has no id")
	}

	return inst.ID, r.ID, nil
}
