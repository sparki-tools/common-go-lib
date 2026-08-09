package airlock

import (
	_ "embed"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

//go:embed testdata/handoff-client.fixture.json
var fixtureBytes []byte

type fixture struct {
	Secret   string `json:"secret"`
	Issuer   string `json:"issuer"`
	Audience string `json:"audience"`
	Subject  string `json:"subject"`
	Payload  struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	} `json:"payload"`
	Token string `json:"token"`
}

func loadFixture(t *testing.T) fixture {
	t.Helper()
	var f fixture
	if err := json.Unmarshal(fixtureBytes, &f); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return f
}

// TestVerifyLocalSession_FixtureFromCanonicalSigner is the cross-language
// contract gate. The token in testdata/ was signed by jose (the canonical
// TS SDK signer); if Go fails to verify it, the Go port has drifted from
// canonical and every backend will reject web-app cookies in production.
func TestVerifyLocalSession_FixtureFromCanonicalSigner(t *testing.T) {
	f := loadFixture(t)

	s, err := VerifyLocalSession(f.Token, f.Issuer, f.Secret)
	if err != nil {
		t.Fatalf("verify canonical-signed token: %v", err)
	}
	if s.UserID != f.Subject {
		t.Errorf("UserID = %q, want %q", s.UserID, f.Subject)
	}
	if s.Email != f.Payload.Email {
		t.Errorf("Email = %q, want %q", s.Email, f.Payload.Email)
	}
	if s.Name != f.Payload.Name {
		t.Errorf("Name = %q, want %q", s.Name, f.Payload.Name)
	}
	if s.Role != f.Payload.Role {
		t.Errorf("Role = %q, want %q", s.Role, f.Payload.Role)
	}
	if !s.ExpiresAt.After(time.Now()) {
		t.Errorf("ExpiresAt %v should be in the future", s.ExpiresAt)
	}
}

func TestVerifyLocalSession_RejectsWrongIssuer(t *testing.T) {
	f := loadFixture(t)
	if _, err := VerifyLocalSession(f.Token, "https://other.tools", f.Secret); err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestVerifyLocalSession_RejectsWrongSecret(t *testing.T) {
	f := loadFixture(t)
	other := strings.Repeat("z", len(f.Secret))
	if _, err := VerifyLocalSession(f.Token, f.Issuer, other); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestVerifyLocalSession_RejectsTamperedToken(t *testing.T) {
	f := loadFixture(t)
	parts := strings.Split(f.Token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}
	// Flip one byte of the signature.
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	parts[2] = string(sig)
	tampered := strings.Join(parts, ".")
	if _, err := VerifyLocalSession(tampered, f.Issuer, f.Secret); err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestVerifyLocalSession_RejectsEmptyToken(t *testing.T) {
	f := loadFixture(t)
	if _, err := VerifyLocalSession("", f.Issuer, f.Secret); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestSignLocalSession_RoundTrip(t *testing.T) {
	secret := strings.Repeat("a", 32)
	issuer := "https://sparki.tools"
	in := Session{
		UserID: "go-user-1",
		Email:  "go@sparki.tools",
		Name:   "Go Tester",
		Role:   "admin",
	}
	tok, err := SignLocalSession(in, issuer, secret, time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := VerifyLocalSession(tok, issuer, secret)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.UserID != in.UserID || got.Email != in.Email || got.Role != in.Role {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestSignLocalSession_RefusesShortSecret(t *testing.T) {
	if _, err := SignLocalSession(Session{UserID: "x", Email: "x@x"}, "https://x", "short", time.Minute); err == nil {
		t.Fatal("expected error for short secret")
	}
}
