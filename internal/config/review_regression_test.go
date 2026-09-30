package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/go-gh/v2/pkg/auth"
)

func TestReviewHostMatchesAuthenticatedGHHost(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte("enterprise.example:\n  user: fixture-user\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_CONFIG_DIR", dir)
	t.Setenv("GH_HOST", "")
	t.Setenv("GH_ENTERPRISE_HOST", "")
	actual, _ := auth.DefaultHost()
	if displayed := DefaultHost(); displayed != actual {
		t.Fatalf("UI/browser host %q differs from API host %q", displayed, actual)
	}
}
