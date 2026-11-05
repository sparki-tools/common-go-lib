package observability_test

import (
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sparki-tools/common-go-lib/observability"
)

// TestRemoteWriteClientLogsNoPasswordMaterial pins the startup log line: it may
// say whether a password is set, and nothing derived from one.
func TestRemoteWriteClientLogsNoPasswordMaterial(t *testing.T) {
	const password = "glc_s3cr3t-value"

	tests := []struct {
		name     string
		password string
		wantSet  bool
	}{
		{name: "password set", password: password, wantSet: true},
		{name: "password unset", password: "", wantSet: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)

			observability.NewRemoteWriteClient(&observability.RemoteWriteConfig{
				Endpoint: "https://prometheus.example/api/prom/push",
				Username: "12345",
				Password: tt.password,
			}, zap.New(core))

			entries := logs.FilterMessage("remote write client configured").All()
			if len(entries) != 1 {
				t.Fatalf("want 1 startup log entry, got %d", len(entries))
			}

			fields := entries[0].ContextMap()
			if got, ok := fields["password_set"]; !ok || got != tt.wantSet {
				t.Errorf("password_set = %v (present %v), want %v", got, ok, tt.wantSet)
			}

			if _, ok := fields["password_preview"]; ok {
				t.Error("password_preview is still logged")
			}

			for key, value := range fields {
				rendered := fmt.Sprint(value)
				if strings.Contains(rendered, password[:4]) {
					t.Errorf("field %q leaks password material: %q", key, rendered)
				}
			}
		})
	}
}
