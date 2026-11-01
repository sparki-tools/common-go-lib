package githubapp_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
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
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sparki-tools/common-go-lib/githubapp"
)

const testAppID int64 = 12345

var (
	keyOnce sync.Once
	testKey *rsa.PrivateKey
)

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}

		testKey = k
	})

	return testKey
}

func pkcs1PEM(t *testing.T) []byte {
	t.Helper()

	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey(t))})
}

func pkcs8PEM(t *testing.T) []byte {
	t.Helper()

	der, err := x509.MarshalPKCS8PrivateKey(rsaKey(t))
	if err != nil {
		t.Fatal(err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// clock is a settable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *clock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// fakeGitHub is a minimal GitHub API: it verifies app JWTs against the public
// key and issues numbered installation tokens valid for one hour.
type fakeGitHub struct {
	t     *testing.T
	clock *clock
	pub   *rsa.PublicKey

	tokenRequests atomic.Int64
	// hook, when set, runs before the default access_tokens handler; returning
	// true means it wrote the response.
	hook func(w http.ResponseWriter, r *http.Request) bool

	mu        sync.Mutex
	bodies    [][]byte
	lastJWT   string
	issued    map[string]bool
	repoAuths []string
}

func newFake(t *testing.T, clk *clock) (*fakeGitHub, *httptest.Server) {
	t.Helper()
	f := &fakeGitHub{t: t, clock: clk, pub: &rsaKey(t).PublicKey, issued: map[string]bool{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	return f, srv
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

	switch {
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/app/installations/"):
		f.handleAccessToken(w, r, auth)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/installation":
		if err := verifyJWT(f.pub, auth, f.clock.Now()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "bad jwt: " + err.Error()})

			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"id": 42, "account": map[string]string{"login": "acme"}})
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget":
		f.mu.Lock()
		ok := f.issued[auth]
		f.repoAuths = append(f.repoAuths, auth)
		f.mu.Unlock()

		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Bad credentials"})

			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"id": 777, "full_name": "acme/widget"})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	}
}

func (f *fakeGitHub) handleAccessToken(w http.ResponseWriter, r *http.Request, auth string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}

	f.mu.Lock()
	f.bodies = append(f.bodies, body)
	f.lastJWT = auth
	f.mu.Unlock()

	if f.hook != nil && f.hook(w, r) {
		return
	}

	if err := verifyJWT(f.pub, auth, f.clock.Now()); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "bad jwt: " + err.Error()})

		return
	}

	n := f.tokenRequests.Add(1)
	tok := fmt.Sprintf("ghs_token_%d", n)

	f.mu.Lock()
	f.issued[tok] = true
	f.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      tok,
		"expires_at": f.clock.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
}

func (f *fakeGitHub) lastBody() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.bodies) == 0 {
		return nil
	}

	return f.bodies[len(f.bodies)-1]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}

type jwtClaims struct {
	Iat int64 `json:"iat"`
	Exp int64 `json:"exp"`
	Iss int64 `json:"iss"`
}

func verifyJWT(pub *rsa.PublicKey, token string, now time.Time) error {
	_, err := parseJWT(pub, token, now)

	return err
}

func parseJWT(pub *rsa.PublicKey, token string, now time.Time) (jwtClaims, error) {
	var claims jwtClaims

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, errors.New("not a JWT")
	}

	enc := base64.RawURLEncoding

	hb, err := enc.DecodeString(parts[0])
	if err != nil {
		return claims, err
	}

	var header map[string]string

	err = json.Unmarshal(hb, &header)
	if err != nil {
		return claims, err
	}

	if header["alg"] != "RS256" || header["typ"] != "JWT" {
		return claims, fmt.Errorf("unexpected header %v", header)
	}

	sig, err := enc.DecodeString(parts[2])
	if err != nil {
		return claims, err
	}

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))

	err = rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig)
	if err != nil {
		return claims, fmt.Errorf("signature: %w", err)
	}

	cb, err := enc.DecodeString(parts[1])
	if err != nil {
		return claims, err
	}

	if err := json.Unmarshal(cb, &claims); err != nil {
		return claims, err
	}

	if now.Unix() < claims.Iat || now.Unix() >= claims.Exp {
		return claims, errors.New("jwt not valid at this time")
	}

	return claims, nil
}

func newClient(t *testing.T, srv *httptest.Server, clk *clock) *githubapp.Client {
	t.Helper()

	c, err := githubapp.New(githubapp.Config{
		AppID:         testAppID,
		PrivateKeyPEM: pkcs1PEM(t),
		BaseURL:       srv.URL + "/",
		HTTPClient:    srv.Client(),
		Now:           clk.Now,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return c
}

func newClock() *clock {
	return &clock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
}

func TestNewKeyFormats(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		appID   int64
		pem     []byte
		wantErr error
		anyErr  bool
	}{
		{name: "pkcs1", appID: testAppID, pem: pkcs1PEM(t)},
		{name: "pkcs8", appID: testAppID, pem: pkcs8PEM(t)},
		{name: "no pem", appID: testAppID, pem: []byte("not a key"), wantErr: githubapp.ErrInvalidKey},
		{
			name: "garbage pkcs1", appID: testAppID, wantErr: githubapp.ErrInvalidKey,
			pem: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("junk")}),
		},
		{
			name: "ec pkcs8", appID: testAppID, wantErr: githubapp.ErrInvalidKey,
			pem: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER}),
		},
		{
			name: "wrong block type", appID: testAppID, wantErr: githubapp.ErrInvalidKey,
			pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("x")}),
		},
		{name: "zero app id", appID: 0, pem: pkcs1PEM(t), anyErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := githubapp.New(githubapp.Config{AppID: tc.appID, PrivateKeyPEM: tc.pem})

			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			case tc.anyErr:
				if err == nil {
					t.Fatal("expected error")
				}
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			}

			if err != nil && strings.Contains(err.Error(), "PRIVATE KEY") && strings.Contains(err.Error(), "-----") {
				t.Fatalf("error leaks key material: %v", err)
			}
		})
	}
}

func TestAppJWTClaimsAndSignature(t *testing.T) {
	for _, format := range []string{"pkcs1", "pkcs8"} {
		t.Run(format, func(t *testing.T) {
			clk := newClock()
			f, srv := newFake(t, clk)

			keyPEM := pkcs1PEM(t)
			if format == "pkcs8" {
				keyPEM = pkcs8PEM(t)
			}

			c, err := githubapp.New(githubapp.Config{
				AppID: testAppID, PrivateKeyPEM: keyPEM, BaseURL: srv.URL, HTTPClient: srv.Client(), Now: clk.Now,
			})
			if err != nil {
				t.Fatal(err)
			}

			if _, err = c.InstallationToken(context.Background(), 1, githubapp.TokenOptions{}); err != nil {
				t.Fatal(err)
			}

			f.mu.Lock()
			jwt := f.lastJWT
			f.mu.Unlock()

			claims, err := parseJWT(f.pub, jwt, clk.Now())
			if err != nil {
				t.Fatalf("verify: %v", err)
			}

			now := clk.Now().Unix()
			if claims.Iat != now-60 {
				t.Errorf("iat = %d, want now-60 (%d)", claims.Iat, now-60)
			}

			if claims.Exp != now+9*60 {
				t.Errorf("exp = %d, want now+540 (%d)", claims.Exp, now+540)
			}

			if claims.Iss != testAppID {
				t.Errorf("iss = %d, want %d", claims.Iss, testAppID)
			}

			other, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}

			if verifyJWT(&other.PublicKey, jwt, clk.Now()) == nil {
				t.Error("JWT verified with an unrelated key")
			}
		})
	}
}

func TestInstallationTokenCache(t *testing.T) {
	clk := newClock()
	f, srv := newFake(t, clk)
	c := newClient(t, srv, clk)
	ctx := context.Background()

	first, err := c.InstallationToken(ctx, 7, githubapp.TokenOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if !first.ExpiresAt.Equal(clk.Now().Add(time.Hour)) {
		t.Fatalf("ExpiresAt = %v", first.ExpiresAt)
	}

	// Just inside the reuse window: cached.
	clk.Set(first.ExpiresAt.Add(-5*time.Minute - time.Second))

	second, err := c.InstallationToken(ctx, 7, githubapp.TokenOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if second.Value != first.Value || f.tokenRequests.Load() != 1 {
		t.Fatalf("expected cache hit, got %q after %d requests", second.Value, f.tokenRequests.Load())
	}

	// At ExpiresAt-5m: refreshed.
	clk.Set(first.ExpiresAt.Add(-5 * time.Minute))

	third, err := c.InstallationToken(ctx, 7, githubapp.TokenOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if third.Value == first.Value || f.tokenRequests.Load() != 2 {
		t.Fatalf("expected refresh, got %q after %d requests", third.Value, f.tokenRequests.Load())
	}

	// The refreshed token is now the cached one.
	fourth, err := c.InstallationToken(ctx, 7, githubapp.TokenOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if fourth.Value != third.Value || f.tokenRequests.Load() != 2 {
		t.Fatalf("refreshed token not cached")
	}
}

func TestInstallationTokenCacheKey(t *testing.T) {
	clk := newClock()
	f, srv := newFake(t, clk)
	c := newClient(t, srv, clk)
	ctx := context.Background()

	get := func(id int64, opts githubapp.TokenOptions) string {
		t.Helper()

		tok, err := c.InstallationToken(ctx, id, opts)
		if err != nil {
			t.Fatal(err)
		}

		return tok.Value
	}

	a := get(1, githubapp.TokenOptions{
		RepositoryIDs: []int64{2, 1}, Permissions: map[string]string{"contents": "read", "checks": "write"}})
	b := get(1, githubapp.TokenOptions{
		RepositoryIDs: []int64{1, 2, 2}, Permissions: map[string]string{"checks": "write", "contents": "read"}})

	if a != b {
		t.Fatal("same scope in different order should share a cache entry")
	}

	distinct := []string{
		a,
		get(2, githubapp.TokenOptions{
			RepositoryIDs: []int64{1, 2}, Permissions: map[string]string{"contents": "read", "checks": "write"}}),
		get(1, githubapp.TokenOptions{
			RepositoryIDs: []int64{1}, Permissions: map[string]string{"contents": "read", "checks": "write"}}),
		get(1, githubapp.TokenOptions{
			RepositoryIDs: []int64{1, 2}, Permissions: map[string]string{"contents": "write", "checks": "write"}}),
		get(1, githubapp.TokenOptions{}),
	}

	seen := map[string]bool{}
	for _, v := range distinct {
		if seen[v] {
			t.Fatalf("distinct scopes shared token %q", v)
		}

		seen[v] = true
	}

	if got := f.tokenRequests.Load(); got != int64(len(distinct)) {
		t.Fatalf("token requests = %d, want %d", got, len(distinct))
	}
}

func TestInstallationTokenRequestBody(t *testing.T) {
	clk := newClock()
	f, srv := newFake(t, clk)
	c := newClient(t, srv, clk)
	ctx := context.Background()

	if _, err := c.InstallationToken(ctx, 9, githubapp.TokenOptions{}); err != nil {
		t.Fatal(err)
	}

	if b := f.lastBody(); len(b) != 0 {
		t.Fatalf("unscoped request sent body %q", b)
	}

	opts := githubapp.TokenOptions{RepositoryIDs: []int64{30, 10, 20}, Permissions: map[string]string{"contents": "read"}}
	if _, err := c.InstallationToken(ctx, 9, opts); err != nil {
		t.Fatal(err)
	}

	var body struct {
		RepositoryIDs []int64           `json:"repository_ids"`
		Permissions   map[string]string `json:"permissions"`
	}

	if err := json.Unmarshal(f.lastBody(), &body); err != nil {
		t.Fatalf("body %q: %v", f.lastBody(), err)
	}

	if fmt.Sprint(body.RepositoryIDs) != "[10 20 30]" || body.Permissions["contents"] != "read" || len(body.Permissions) != 1 {
		t.Fatalf("unexpected body %+v", body)
	}

	if fmt.Sprint(opts.RepositoryIDs) != "[30 10 20]" {
		t.Fatal("caller's RepositoryIDs slice was mutated")
	}

	if _, err := c.InstallationToken(ctx, 9, githubapp.TokenOptions{Permissions: map[string]string{"metadata": "read"}}); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(f.lastBody()), "repository_ids") {
		t.Fatalf("permissions-only body carries repository_ids: %s", f.lastBody())
	}
}

func TestInstallationTokenSingleflight(t *testing.T) {
	clk := newClock()
	f, srv := newFake(t, clk)
	c := newClient(t, srv, clk)

	entered := make(chan struct{}, 100)
	release := make(chan struct{})
	f.hook = func(http.ResponseWriter, *http.Request) bool {
		entered <- struct{}{}

		<-release

		return false
	}

	const callers = 25

	var wg sync.WaitGroup

	results := make([]githubapp.Token, callers)
	errs := make([]error, callers)

	for i := range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			results[i], errs[i] = c.InstallationToken(context.Background(), 5, githubapp.TokenOptions{})
		}()
	}

	<-entered
	time.Sleep(50 * time.Millisecond) // let the other callers queue on the in-flight request
	close(release)
	wg.Wait()

	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}

		if results[i].Value != results[0].Value {
			t.Fatalf("caller %d got %q, caller 0 got %q", i, results[i].Value, results[0].Value)
		}
	}

	if got := f.tokenRequests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}

	if len(entered) != 0 {
		t.Fatalf("%d extra requests reached the server", len(entered))
	}
}

func TestInstallationTokenFollowerSurvivesLeaderCancel(t *testing.T) {
	clk := newClock()
	f, srv := newFake(t, clk)
	c := newClient(t, srv, clk)

	var calls atomic.Int64

	entered := make(chan struct{}, 2)
	f.hook = func(_ http.ResponseWriter, r *http.Request) bool {
		if calls.Add(1) == 1 {
			entered <- struct{}{}

			<-r.Context().Done() // hold the leader's request until it is canceled
		}

		return false
	}

	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)

	go func() {
		_, err := c.InstallationToken(leaderCtx, 3, githubapp.TokenOptions{})
		leaderErr <- err
	}()

	<-entered

	followerDone := make(chan error, 1)

	var follower githubapp.Token

	go func() {
		var err error
		follower, err = c.InstallationToken(context.Background(), 3, githubapp.TokenOptions{})
		followerDone <- err
	}()

	time.Sleep(50 * time.Millisecond) // let the follower join the in-flight call
	cancel()

	if err := <-leaderErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader err = %v, want context.Canceled", err)
	}

	if err := <-followerDone; err != nil {
		t.Fatalf("follower err = %v, want a token", err)
	}

	if follower.Value == "" {
		t.Fatal("follower got empty token")
	}
}

func TestErrorMapping(t *testing.T) {
	reset := time.Date(2026, 11, 1, 12, 10, 0, 0, time.UTC)

	tests := []struct {
		name       string
		status     int
		headers    map[string]string
		want       error
		retryAfter time.Duration
	}{
		{name: "404 not installed", status: http.StatusNotFound, want: githubapp.ErrNotInstalled},
		{name: "401 unauthorized", status: http.StatusUnauthorized, want: githubapp.ErrUnauthorized},
		{
			name: "429 retry-after", status: http.StatusTooManyRequests, want: githubapp.ErrRateLimited,
			headers: map[string]string{"Retry-After": "30"}, retryAfter: 30 * time.Second,
		},
		{
			name: "403 primary limit", status: http.StatusForbidden, want: githubapp.ErrRateLimited,
			headers:    map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10)},
			retryAfter: 10 * time.Minute,
		},
		{
			name: "403 secondary limit", status: http.StatusForbidden, want: githubapp.ErrRateLimited,
			headers: map[string]string{"Retry-After": "60"}, retryAfter: time.Minute,
		},
		{name: "403 plain", status: http.StatusForbidden},
		{name: "422", status: http.StatusUnprocessableEntity},
	}

	sentinels := []error{githubapp.ErrNotInstalled, githubapp.ErrUnauthorized, githubapp.ErrRateLimited}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := newClock()
			f, srv := newFake(t, clk)
			c := newClient(t, srv, clk)

			f.hook = func(w http.ResponseWriter, _ *http.Request) bool {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}

				writeJSON(w, tc.status, map[string]string{"message": "nope"})

				return true
			}

			_, err := c.InstallationToken(context.Background(), 11, githubapp.TokenOptions{})
			if err == nil {
				t.Fatal("expected error")
			}

			for _, s := range sentinels {
				if got := errors.Is(err, s); got != (errors.Is(s, tc.want)) {
					t.Errorf("errors.Is(err, %v) = %v; err = %v", s, got, err)
				}
			}

			var apiErr *githubapp.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("errors.As APIError failed: %v", err)
			}

			if apiErr.StatusCode != tc.status || apiErr.Message != "nope" || apiErr.RetryAfter != tc.retryAfter {
				t.Errorf("APIError = %+v", apiErr)
			}

			// Failures are not cached: the next call goes back to GitHub.
			f.hook = nil

			if _, err := c.InstallationToken(context.Background(), 11, githubapp.TokenOptions{}); err != nil {
				t.Fatalf("retry after error: %v", err)
			}
		})
	}
}

func TestCredentialsNeverInErrors(t *testing.T) {
	clk := newClock()
	f, srv := newFake(t, clk)
	c := newClient(t, srv, clk)
	ctx := context.Background()

	// The server echoes the bearer credential back in its error message.
	echo := func(w http.ResponseWriter, r *http.Request) bool {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "rejected " + r.Header.Get("Authorization")})

		return true
	}
	f.hook = echo

	_, err := c.InstallationToken(ctx, 1, githubapp.TokenOptions{})
	if err == nil {
		t.Fatal("expected error")
	}

	f.mu.Lock()
	jwt := f.lastJWT
	f.mu.Unlock()

	if jwt == "" || strings.Contains(err.Error(), jwt) || strings.Contains(fmt.Sprintf("%+v", err), jwt) {
		t.Fatalf("JWT leaked into error: %v", err)
	}

	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("expected redaction marker in %q", err)
	}

	// Installation token echoed by the repo endpoint.
	f.hook = nil

	tok, err := c.InstallationToken(ctx, 42, githubapp.TokenOptions{Permissions: map[string]string{"metadata": "read"}})
	if err != nil {
		t.Fatal(err)
	}

	f.mu.Lock()
	delete(f.issued, tok.Value) // make the repo endpoint reject it
	f.mu.Unlock()

	echoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/widget" {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "no " + r.Header.Get("Authorization")})

			return
		}

		f.ServeHTTP(w, r)
	}))
	t.Cleanup(echoSrv.Close)

	c2 := newClient(t, echoSrv, clk)

	_, _, err = c2.Installation(ctx, "acme", "widget")
	if !errors.Is(err, githubapp.ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}

	f.mu.Lock()

	issued := make([]string, 0, len(f.issued))
	for k := range f.issued {
		issued = append(issued, k)
	}
	f.mu.Unlock()

	for _, v := range issued {
		if strings.Contains(err.Error(), v) {
			t.Fatalf("installation token leaked into error: %v", err)
		}
	}

	// Token formatting redacts the value.
	for _, s := range []string{tok.String(), tok.String(), fmt.Sprintf("%+v", tok), fmt.Sprintf("%#v", tok)} {
		if strings.Contains(s, tok.Value) {
			t.Fatalf("Token formatting leaked value: %s", s)
		}
	}
}

func TestInstallation(t *testing.T) {
	clk := newClock()
	f, srv := newFake(t, clk)
	c := newClient(t, srv, clk)

	instID, repoID, err := c.Installation(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatal(err)
	}

	if instID != 42 || repoID != 777 {
		t.Fatalf("got (%d, %d), want (42, 777)", instID, repoID)
	}

	if !strings.Contains(string(f.lastBody()), `"metadata":"read"`) {
		t.Fatalf("repo lookup token not scoped to metadata:read: %s", f.lastBody())
	}

	f.mu.Lock()
	auths := append([]string(nil), f.repoAuths...)
	f.mu.Unlock()

	if len(auths) != 1 || !strings.HasPrefix(auths[0], "ghs_") {
		t.Fatalf("repo lookup did not use an installation token: %v", auths)
	}

	// A second lookup reuses the cached metadata token.
	if _, _, err := c.Installation(context.Background(), "acme", "widget"); err != nil {
		t.Fatal(err)
	}

	if got := f.tokenRequests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
}

func TestInstallationNotInstalled(t *testing.T) {
	clk := newClock()
	f, srv := newFake(t, clk)
	c := newClient(t, srv, clk)

	_, _, err := c.Installation(context.Background(), "acme", "missing")
	if !errors.Is(err, githubapp.ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}

	if f.tokenRequests.Load() != 0 {
		t.Fatal("minted a token for a repo without an installation")
	}

	if _, _, err := c.Installation(context.Background(), "", "widget"); err == nil {
		t.Fatal("expected error for empty owner")
	}
}

func TestInstallationTokenContextCancelled(t *testing.T) {
	clk := newClock()
	_, srv := newFake(t, clk)
	c := newClient(t, srv, clk)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.InstallationToken(ctx, 1, githubapp.TokenOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	if _, err := c.InstallationToken(context.Background(), 0, githubapp.TokenOptions{}); err == nil {
		t.Fatal("expected error for zero installation ID")
	}
}
