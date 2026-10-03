package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsMissingClerkSecretKey(t *testing.T) {
	// Arrange: remove the server-side Clerk secret from the environment.
	t.Setenv("CLERK_SECRET_KEY", "")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/chattered?sslmode=disable")
	t.Setenv("PORT", "8080")

	// Act: load the server configuration.
	_, err := Load()

	// Assert: startup fails before protected routes can accept requests.
	if err == nil || !strings.Contains(err.Error(), "CLERK_SECRET_KEY") {
		t.Fatalf("Load() error = %v, want a missing Clerk secret error", err)
	}
}

func TestLoadTrimsClerkSecretKey(t *testing.T) {
	// Arrange: provide a secret with accidental surrounding whitespace.
	t.Setenv("CLERK_SECRET_KEY", "  sk_test_example  ")
	t.Setenv("DATABASE_URL", "  postgres://user:pass@localhost:5432/chattered?sslmode=disable  ")
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")

	// Act: load the server configuration.
	config, err := Load()

	// Assert: the secret is normalized and the default port is applied.
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if config.ClerkSecretKey != "sk_test_example" || config.DatabaseURL != "postgres://user:pass@localhost:5432/chattered?sslmode=disable" || config.Host != "0.0.0.0" || config.Port != "8080" {
		t.Errorf("config = %#v, want trimmed secrets and default listen address", config)
	}
}

func TestLoadTrimsConfiguredListenAddress(t *testing.T) {
	// Arrange: configure an explicit host and port with surrounding whitespace.
	t.Setenv("CLERK_SECRET_KEY", "sk_test_example")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/chattered?sslmode=disable")
	t.Setenv("HOST", "  127.0.0.1  ")
	t.Setenv("PORT", "  9090  ")

	// Act: load the server configuration.
	config, err := Load()

	// Assert: the explicit listen address is normalized.
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if config.Host != "127.0.0.1" || config.Port != "9090" {
		t.Errorf("listen address = %s:%s, want 127.0.0.1:9090", config.Host, config.Port)
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	// Arrange: configure Clerk but omit the database connection string.
	t.Setenv("CLERK_SECRET_KEY", "sk_test_example")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PORT", "8080")

	// Act: load the server configuration.
	_, err := Load()

	// Assert: startup fails with an actionable database configuration error.
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load() error = %v, want a missing database URL error", err)
	}
}
