package config

import "testing"

func TestDefaultHostEnv(t *testing.T) {
	t.Setenv("GH_HOST", "Example.COM/")
	t.Setenv("GH_ENTERPRISE_HOST", "enterprise.example")
	if got := DefaultHost(); got != "example.com" {
		t.Fatalf("host = %q", got)
	}
	t.Setenv("GH_HOST", "")
	if got := DefaultHost(); got != "enterprise.example" {
		t.Fatalf("enterprise host = %q", got)
	}
	if got := normalizeHost(""); got != "github.com" {
		t.Fatalf("empty host = %q", got)
	}
}
