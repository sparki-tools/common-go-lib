package airlock

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Mirrors devarno-cloud/sdk-js/src/otel/auth-span.ts. Keep field names and
// the closed reason enum aligned with that contract — atlas's
// auth-span-schema.yaml validates exporters against this exact shape.

type AuthDecisionOutcome string

const (
	OutcomeAllow    AuthDecisionOutcome = "allow"
	OutcomeDeny     AuthDecisionOutcome = "deny"
	OutcomeRedirect AuthDecisionOutcome = "redirect"
)

type AuthDecisionReason string

const (
	ReasonValidSession         AuthDecisionReason = "valid_session"
	ReasonNoCookie             AuthDecisionReason = "no_cookie"
	ReasonInvalidSession       AuthDecisionReason = "invalid_session"
	ReasonExpired              AuthDecisionReason = "expired"
	ReasonRevoked              AuthDecisionReason = "revoked"
	ReasonMissingRole          AuthDecisionReason = "missing_role"
	ReasonMissingOrgMembership AuthDecisionReason = "missing_org_membership"
	ReasonTierDenied           AuthDecisionReason = "tier_denied"
	ReasonRoleDenied           AuthDecisionReason = "role_denied"
	ReasonDisabledBypass       AuthDecisionReason = "disabled_bypass"
	ReasonTampered             AuthDecisionReason = "tampered"
	ReasonNetworkError         AuthDecisionReason = "network_error"
	ReasonAPIKeyInvalid        AuthDecisionReason = "api_key_invalid"
	ReasonAPIKeyRevoked        AuthDecisionReason = "api_key_revoked"
	ReasonMCPScopeDenied       AuthDecisionReason = "mcp_scope_denied"
	ReasonWSUpgradeNoToken     AuthDecisionReason = "ws_upgrade_missing_token"
	ReasonOther                AuthDecisionReason = "other"
)

type AuthTruthSource string

const (
	TruthSourceAirlock        AuthTruthSource = "airlock"
	TruthSourceCasaBetterAuth AuthTruthSource = "casa-betterauth"
	TruthSourceBoth           AuthTruthSource = "both"
)

// AuthDecisionAttrs is the closed attribute set for an auth.decision span.
// Refuses email/name/ip/ua even via the attrs path — mirrors the TS PII policy.
type AuthDecisionAttrs struct {
	Flow                string
	Decision            AuthDecisionOutcome
	Reason              AuthDecisionReason
	Sub                 string // empty = unknown / pre-lookup
	Scope               string // empty = unresolved
	TruthSource         AuthTruthSource
	Client              string
	RevocationLatencyMs *int64
	SessionAgeMs        *int64
}

// EmitAuthDecision records a single point-in-time auth.decision span.
// Caller-supplied flow id is required (e.g. "F9" for cross-apex). When no
// tracer provider is registered, this is a no-op and safe to ship before
// the OTel exporter pipeline is wired (matches the TS helper).
func EmitAuthDecision(ctx context.Context, a AuthDecisionAttrs) {
	tracer := otel.GetTracerProvider().Tracer("devarno.auth")
	_, span := tracer.Start(ctx, "auth.decision")
	span.SetAttributes(
		attribute.String("auth.flow", a.Flow),
		attribute.String("auth.decision", string(a.Decision)),
		attribute.String("auth.reason", string(a.Reason)),
	)
	if a.Sub != "" {
		span.SetAttributes(attribute.String("auth.sub", a.Sub))
	}
	if a.Scope != "" {
		span.SetAttributes(attribute.String("auth.scope", a.Scope))
	}
	if a.TruthSource != "" {
		span.SetAttributes(attribute.String("auth.truth_source", string(a.TruthSource)))
	}
	if a.Client != "" {
		span.SetAttributes(attribute.String("auth.client", a.Client))
	}
	if a.RevocationLatencyMs != nil {
		span.SetAttributes(attribute.Int64("auth.revocation_latency_ms", *a.RevocationLatencyMs))
	}
	if a.SessionAgeMs != nil {
		span.SetAttributes(attribute.Int64("auth.session_age_ms", *a.SessionAgeMs))
	}

	if a.Decision == OutcomeAllow {
		span.SetStatus(codes.Ok, "")
	} else {
		span.SetStatus(codes.Error, string(a.Reason))
	}
	span.End()
}
