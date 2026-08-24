package config

import (
	"strings"
	"testing"
)

func TestLoadRequiresBootstrapSecret(t *testing.T) {
	t.Setenv("BRIDGEWATCH_BOOTSTRAP_PASSWORD", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "BRIDGEWATCH_BOOTSTRAP_PASSWORD") {
		t.Fatalf("Load() error = %v, want required bootstrap password", err)
	}
}

func TestLoadAcceptsCompleteEnvironment(t *testing.T) {
	t.Setenv("BRIDGEWATCH_BOOTSTRAP_PASSWORD", "foundation-test-secret")
	t.Setenv("BRIDGEWATCH_ADDR", ":9090")
	t.Setenv("BRIDGEWATCH_DATABASE", "/tmp/bridgewatch-config-test.db")
	t.Setenv("BRIDGEWATCH_SESSION_TTL", "2h")
	t.Setenv("BRIDGEWATCH_WORKER_INTERVAL", "250ms")
	t.Setenv("BRIDGEWATCH_SHUTDOWN_TIMEOUT", "5s")
	t.Setenv("BRIDGEWATCH_MAX_REQUEST_BYTES", "65536")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Address != ":9090" || cfg.DatabasePath != "/tmp/bridgewatch-config-test.db" || cfg.MaxRequestBytes != 65536 {
		t.Fatalf("Load() config = %+v", cfg)
	}
}
