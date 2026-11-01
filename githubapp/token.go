package githubapp

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TokenOptions narrows an installation token. Zero value means the
// installation's full repository set and permissions.
type TokenOptions struct {
	// RepositoryIDs limits the token to these repositories.
	RepositoryIDs []int64
	// Permissions limits the token to these permissions, e.g. {"contents": "read"}.
	Permissions map[string]string
}

// Token is an installation access token. Value is a credential: do not log it.
type Token struct {
	Value     string
	ExpiresAt time.Time
}

// String redacts the token value so it cannot leak through %v or %s.
func (t Token) String() string {
	return "githubapp.Token{Value:[REDACTED], ExpiresAt:" + t.ExpiresAt.UTC().Format(time.RFC3339) + "}"
}

// GoString redacts the token value for %#v.
func (t Token) GoString() string { return t.String() }

type accessTokenRequest struct {
	RepositoryIDs []int64           `json:"repository_ids,omitempty"`
	Permissions   map[string]string `json:"permissions,omitempty"`
}

// call is one in-flight token request shared by concurrent callers.
type call struct {
	done chan struct{}
	tok  Token
	err  error
}

// InstallationToken returns an installation access token for installationID,
// narrowed by opts. Tokens are cached per (installation, repository IDs,
// permissions) and reused until five minutes before they expire. Concurrent
// cache misses for the same key share a single request to GitHub.
func (c *Client) InstallationToken(ctx context.Context, installationID int64, opts TokenOptions) (Token, error) {
	if installationID <= 0 {
		return Token{}, errors.New("githubapp: installationID must be positive")
	}

	repoIDs := slices.Clone(opts.RepositoryIDs)
	slices.Sort(repoIDs)
	repoIDs = slices.Compact(repoIDs)
	perms := maps.Clone(opts.Permissions)
	key := cacheKey(installationID, repoIDs, perms)

	for {
		c.mu.Lock()

		if tok, ok := c.cache[key]; ok && c.now().Before(tok.ExpiresAt.Add(-tokenRefreshMargin)) {
			c.mu.Unlock()

			return tok, nil
		}

		if cl, ok := c.inflight[key]; ok {
			c.mu.Unlock()

			select {
			case <-cl.done:
			case <-ctx.Done():
				return Token{}, ctx.Err()
			}

			// The leader's context ending is not our failure: retry while ours is live.
			if isContextErr(cl.err) && ctx.Err() == nil {
				continue
			}

			return cl.tok, cl.err
		}

		cl := &call{done: make(chan struct{})}
		c.inflight[key] = cl
		c.mu.Unlock()

		cl.tok, cl.err = c.fetchToken(ctx, installationID, repoIDs, perms)

		c.mu.Lock()
		delete(c.inflight, key)

		if cl.err == nil {
			c.cache[key] = cl.tok
		}
		c.mu.Unlock()
		close(cl.done)

		return cl.tok, cl.err
	}
}

func (c *Client) fetchToken(ctx context.Context, installationID int64, repoIDs []int64, perms map[string]string) (Token, error) {
	jwt, err := c.appJWT()
	if err != nil {
		return Token{}, err
	}

	// A nil body sends no payload: GitHub then grants the installation's full scope.
	var reqBody any
	if len(repoIDs) > 0 || len(perms) > 0 {
		reqBody = accessTokenRequest{RepositoryIDs: repoIDs, Permissions: perms}
	}

	var resp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	path := "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens"

	if err := c.do(ctx, http.MethodPost, path, jwt, reqBody, &resp); err != nil {
		return Token{}, err
	}

	if resp.Token == "" || resp.ExpiresAt.IsZero() {
		return Token{}, errors.New("githubapp: access token response missing token or expires_at")
	}

	return Token{Value: resp.Token, ExpiresAt: resp.ExpiresAt}, nil
}

// cacheKey is canonical: repoIDs must be sorted; permissions are sorted here.
func cacheKey(installationID int64, repoIDs []int64, perms map[string]string) string {
	var b strings.Builder

	b.WriteString(strconv.FormatInt(installationID, 10))
	b.WriteString("|")

	for i, id := range repoIDs {
		if i > 0 {
			b.WriteString(",")
		}

		b.WriteString(strconv.FormatInt(id, 10))
	}

	b.WriteString("|")

	for i, k := range slices.Sorted(maps.Keys(perms)) {
		if i > 0 {
			b.WriteString(",")
		}

		b.WriteString(k + "=" + perms[k])
	}

	return b.String()
}

func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
