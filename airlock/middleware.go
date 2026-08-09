package airlock

import (
	"github.com/gofiber/fiber/v2"
)

// LocalCookieName is the name of the apex-local session cookie set by
// web-app's /auth/callback (matches the canonical SDK constant).
const LocalCookieName = "__app_session"

// localsKey is the Fiber locals key under which the verified Session is
// stored. Use UserFromContext to read it — handlers should not depend on
// the literal key string.
const localsKey = "airlock.session"

// Options configures RequireSession.
type Options struct {
	// Issuer is the consuming apex — must match what web-app passed to
	// signLocalSession. Required.
	Issuer string
	// Secret is the per-app HS256 signing secret (≥32 chars). Required.
	Secret string
	// Client is the slug of this service (e.g. "sparki-api") — appears as
	// auth.client on emitted spans. Required.
	Client string
	// Skipper, if non-nil and returning true, lets a request bypass auth.
	// Use for /health, /metrics, /docs.
	Skipper func(c *fiber.Ctx) bool
	// AllowAPIKey, if non-nil, lets a request authenticate via an alternate
	// path before falling back to the cookie. Returning a non-empty user id
	// counts as authenticated; the caller is responsible for emitting any
	// auth.decision span for that branch. Used by smithy in Phase 1 to keep
	// X-API-Key working alongside the new cookie. May be nil.
	AllowAPIKey func(c *fiber.Ctx) (*Session, bool)
}

// RequireSession enforces a valid apex-local session on every request that
// passes the optional Skipper. On success, stores the Session in fiber locals
// (read with UserFromContext) and emits an allow auth.decision span. On
// failure, emits a deny span and returns 401 with a JSON error body.
func RequireSession(opts Options) fiber.Handler {
	if opts.Issuer == "" || opts.Secret == "" || opts.Client == "" {
		panic("airlock.RequireSession: Issuer, Secret, and Client are required")
	}
	return func(c *fiber.Ctx) error {
		if opts.Skipper != nil && opts.Skipper(c) {
			return c.Next()
		}

		// API-key fallback (transitional — see Options.AllowAPIKey).
		if opts.AllowAPIKey != nil {
			if session, ok := opts.AllowAPIKey(c); ok && session != nil {
				c.Locals(localsKey, session)
				propagateUserHeaders(c, session)
				return c.Next()
			}
		}

		raw := c.Cookies(LocalCookieName)
		if raw == "" {
			EmitAuthDecision(c.UserContext(), AuthDecisionAttrs{
				Flow: "F9", Decision: OutcomeDeny, Reason: ReasonNoCookie,
				TruthSource: TruthSourceAirlock, Client: opts.Client,
			})
			return unauthorized(c, "missing_session")
		}

		session, err := VerifyLocalSession(raw, opts.Issuer, opts.Secret)
		if err != nil || session == nil {
			EmitAuthDecision(c.UserContext(), AuthDecisionAttrs{
				Flow: "F9", Decision: OutcomeDeny, Reason: ReasonInvalidSession,
				TruthSource: TruthSourceAirlock, Client: opts.Client,
			})
			return unauthorized(c, "invalid_session")
		}

		EmitAuthDecision(c.UserContext(), AuthDecisionAttrs{
			Flow: "F9", Decision: OutcomeAllow, Reason: ReasonValidSession,
			Sub: session.UserID, Scope: session.Role,
			TruthSource: TruthSourceAirlock, Client: opts.Client,
		})

		c.Locals(localsKey, session)
		propagateUserHeaders(c, session)
		return c.Next()
	}
}

// UserFromContext returns the verified Session attached by RequireSession.
// Returns nil, false when called from a handler outside the middleware
// chain or when the request was authenticated via a non-session path that
// did not populate locals.
func UserFromContext(c *fiber.Ctx) (*Session, bool) {
	v := c.Locals(localsKey)
	if v == nil {
		return nil, false
	}
	s, ok := v.(*Session)
	return s, ok
}

// propagateUserHeaders surfaces identity to downstream services and to
// observability tooling (rate-limit consumers key off X-User-Id when Kong
// stops issuing JWT-based consumers — see plan §1f).
func propagateUserHeaders(c *fiber.Ctx, s *Session) {
	c.Set("X-User-Id", s.UserID)
	c.Set("X-User-Email", s.Email)
	c.Set("X-User-Role", s.Role)
}

func unauthorized(c *fiber.Ctx, reason string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": reason})
}

// RequireRole gates a route to sessions whose Role is one of the supplied
// values. Must run after RequireSession in the middleware chain.
func RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		s, ok := UserFromContext(c)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no_session"})
		}
		for _, r := range roles {
			if s.Role == r {
				return c.Next()
			}
		}
		EmitAuthDecision(c.UserContext(), AuthDecisionAttrs{
			Flow: "F9", Decision: OutcomeDeny, Reason: ReasonRoleDenied,
			Sub: s.UserID, Scope: s.Role, TruthSource: TruthSourceAirlock,
		})
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "role_denied"})
	}
}

// RequireAdmin is sugar for RequireRole("admin").
func RequireAdmin() fiber.Handler {
	return RequireRole("admin")
}
