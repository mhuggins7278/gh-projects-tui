package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GH_PROJECTS_TUI_CONFIG_DIR", dir)
	t.Setenv("GH_HOST", "")
	t.Setenv("GH_ENTERPRISE_HOST", "")
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withTempConfig(t)
	if err := Save("github.com", Selection{Owner: "org", Project: 7, View: 3}); err != nil {
		t.Fatal(err)
	}
	got, err := Load("github.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != (Selection{Owner: "org", Project: 7, View: 3}) {
		t.Fatalf("selection = %#v", got)
	}
	// Host scoping.
	other, err := Load("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if other != (Selection{}) {
		t.Fatalf("other host = %#v", other)
	}
}

func TestSaveNormalizesDescendants(t *testing.T) {
	withTempConfig(t)
	if err := Save("github.com", Selection{Owner: "", Project: 7, View: 3}); err != nil {
		t.Fatal(err)
	}
	got, _ := Load("github.com")
	if got != (Selection{}) {
		t.Fatalf("cleared = %#v", got)
	}
}

func TestResolveAncestorDiscard(t *testing.T) {
	remembered := Selection{Owner: "old", Project: 7, View: 3}
	if got := Resolve(Selection{Owner: "new"}, remembered); got != (Selection{Owner: "new"}) {
		t.Fatalf("owner change = %#v", got)
	}
	if got := Resolve(Selection{Owner: "new", Project: 9}, remembered); got != (Selection{Owner: "new", Project: 9}) {
		t.Fatalf("owner+project = %#v", got)
	}
	if got := Resolve(Selection{Project: 9}, remembered); got != (Selection{Owner: "old", Project: 9}) {
		t.Fatalf("project change drops view = %#v", got)
	}
	if got := Resolve(Selection{}, remembered); got != remembered {
		t.Fatalf("empty explicit keeps remembered = %#v", got)
	}
}

func TestDefaultHostEnv(t *testing.T) {
	withTempConfig(t)
	t.Setenv("GH_HOST", "Example.COM/")
	if got := DefaultHost(); got != "example.com" {
		t.Fatalf("host = %q", got)
	}
}

func TestConfigFilePermissions(t *testing.T) {
	withTempConfig(t)
	if err := Save("github.com", Selection{Owner: "org"}); err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("GH_PROJECTS_TUI_CONFIG_DIR")
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o", info.Mode().Perm())
	}
}

func TestRememberedFallbackSelection(t *testing.T) {
	withTempConfig(t)
	want := Selection{Owner: "org", Project: 7, Fallback: true}
	if err := Save("github.com", want); err != nil {
		t.Fatal(err)
	}
	got, err := Load("github.com")
	if err != nil || got != want {
		t.Fatalf("fallback selection = %#v, err = %v", got, err)
	}
	if got := Resolve(Selection{}, want); got != want {
		t.Fatalf("remembered fallback = %#v", got)
	}
	if got := Resolve(Selection{Project: 8}, want); got != (Selection{Owner: "org", Project: 8}) {
		t.Fatalf("project change retained fallback = %#v", got)
	}
	if got := Resolve(Selection{View: 3}, want); got != (Selection{Owner: "org", Project: 7, View: 3}) {
		t.Fatalf("saved view selection retained fallback = %#v", got)
	}
}
