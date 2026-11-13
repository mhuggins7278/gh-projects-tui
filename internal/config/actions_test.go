package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinActionsConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	dir, err := actionsConfigDir("darwin")
	if err != nil || dir != filepath.Join(home, ".config") {
		t.Fatalf("default dir=%q err=%v", dir, err)
	}
	xdg := filepath.Join(home, "custom-config")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir, err = actionsConfigDir("darwin")
	if err != nil || dir != xdg {
		t.Fatalf("XDG dir=%q err=%v", dir, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative")
	if _, err := actionsConfigDir("darwin"); err == nil {
		t.Fatal("relative XDG_CONFIG_HOME was accepted")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if _, err := actionsConfigDir("darwin"); err == nil {
		t.Fatal("missing HOME was accepted")
	}
}

func TestLoadActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if actions, err := LoadActions(path, false); err != nil || len(actions) != 0 {
		t.Fatalf("missing optional file: %#v %v", actions, err)
	}
	if _, err := LoadActions(path, true); err == nil {
		t.Fatal("missing explicit file was accepted")
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if actions, err := LoadActions(path, true); err != nil || len(actions) != 0 {
		t.Fatalf("empty file: %#v %v", actions, err)
	}
	data := `actions:
  - key: ctrl+p
    name: Pin
    on: issue
    run: [gh, issue, pin, "{{.IssueNumber}}", --repo, "{{.RepoName}}"]
  - key: ctrl+t
    name: View PR
    on: pr
    run: [gh, pr, view, "{{.PrNumber}}"]
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	actions, err := LoadActions(path, true)
	if err != nil || len(actions) != 2 || actions[0].Run[3] != "{{.IssueNumber}}" || actions[1].On != "pr" {
		t.Fatalf("actions=%#v err=%v", actions, err)
	}
}

func TestLoadActionsRejectsInvalidConfigurations(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{"conflicting key", "actions:\n  - {key: j, name: Test, on: issue, run: [true]}\n", "reserved"},
		{"terminal alias", "actions:\n  - {key: ctrl+m, name: Test, on: issue, run: [true]}\n", "reserved"},
		{"duplicate", "actions:\n  - {key: ctrl+p, name: One, on: issue, run: [true]}\n  - {key: ctrl+p, name: Two, on: pr, run: [true]}\n", "duplicate"},
		{"unknown field", "actions:\n  - {key: ctrl+p, name: One, on: issue, command: true}\n", "field command"},
		{"missing scope", "actions:\n  - {key: ctrl+p, name: One, run: [true]}\n", "on must"},
		{"no executable", "actions:\n  - {key: ctrl+p, name: One, on: issue, run: []}\n", "executable"},
		{"unknown placeholder", "actions:\n  - {key: ctrl+p, name: One, on: issue, run: ['echo', '{{.Nope}}']}\n", "invalid argument"},
		{"multiple documents", "actions: []\n---\nactions: []\n", "multiple YAML documents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadActions(path, true); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want %s", err, tc.want)
			}
		})
	}
}
