package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func checkoutForTest(t *testing.T, path, remote string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", path}, {"-C", path, "remote", "add", "upstream", remote}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return path
}

func TestResolveRepoPathMappingsAndCurrentCheckout(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", base)
	current := checkoutForTest(t, filepath.Join(base, "current"), "git@github.com:Owner/Repo.git")
	mapped := checkoutForTest(t, filepath.Join(base, "mapped", "Repo"), "https://github.com/owner/repo.git")
	exact := checkoutForTest(t, filepath.Join(base, "special"), "ssh://git@github.com/OWNER/REPO.git")
	paths := map[string]string{"owner/*": "~/mapped/*", "owner/repo": "~/special"}
	got, err := ResolveRepoPath(context.Background(), "github.com", "Owner/Repo", current, paths, nil)
	if err != nil || got != exact {
		t.Fatalf("exact: %q %v", got, err)
	}
	delete(paths, "owner/repo")
	got, err = ResolveRepoPath(context.Background(), "github.com", "Owner/Repo", current, paths, nil)
	if err != nil || got != mapped {
		t.Fatalf("wildcard: %q %v", got, err)
	}
	got, err = ResolveRepoPath(context.Background(), "github.com", "Owner/Repo", current, nil, nil)
	if err != nil || got != current {
		t.Fatalf("current: %q %v", got, err)
	}
	missing := filepath.Join(base, "not-yet", "Repo")
	got, err = ResolveRepoPath(context.Background(), "github.com", "Owner/Repo", current, map[string]string{"owner/*": filepath.Join(base, "not-yet", "*")}, nil)
	if err != nil || got != missing {
		t.Fatalf("missing mapped path for clone helper: %q %v", got, err)
	}
	if _, err := ResolveRepoPath(context.Background(), "other.example", "Owner/Repo", current, paths, nil); err == nil || !strings.Contains(err.Error(), "mapped path") {
		t.Fatalf("wrong host accepted: %v", err)
	}
}

func TestResolveRepoPathSSHAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a shell script for the SSH executable")
	}
	bin := t.TempDir()
	ssh := `#!/bin/sh
[ "$1" = "-G" ] && [ "$2" = "--" ] || exit 1
case "$3" in
    git@github-work|ssh://git@github-work:443|github-work) echo 'hostname github.com' ;;
    git@other-host) echo 'hostname other.example' ;;
    *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tt := range []struct {
		name   string
		remote string
		match  bool
	}{
		{"SCP", "git@github-work:owner/repo.git", true},
		{"SSH URL", "ssh://git@github-work:443/owner/repo.git", true},
		{"HTTPS", "https://github-work/owner/repo.git", false},
		{"wrong host", "git@other-host:owner/repo.git", false},
		{"wrong repository", "git@github-work:other/repo.git", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkout := checkoutForTest(t, t.TempDir(), tt.remote)
			got, err := ResolveRepoPath(context.Background(), "github.com", "owner/repo", "", map[string]string{"owner/repo": checkout}, nil)
			if tt.match && (err != nil || got != checkout) || !tt.match && err == nil {
				t.Fatalf("ResolveRepoPath with %q returned %q, %v", tt.remote, got, err)
			}
		})
	}
}

func TestResolveRepoPathSearchRootsAndAmbiguity(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "projects")
	first := checkoutForTest(t, filepath.Join(root, "different-name"), "https://github.com/owner/repo")
	checkoutForTest(t, filepath.Join(root, "not-the-repo"), "git@github.com:other/repo.git")
	got, err := ResolveRepoPath(context.Background(), "github.com", "owner/repo", base, nil, []string{root})
	if err != nil || got != first {
		t.Fatalf("search root: %q %v", got, err)
	}
	checkoutForTest(t, filepath.Join(root, "owner", "another-name"), "git@github.com:owner/repo.git")
	if _, err := ResolveRepoPath(context.Background(), "github.com", "owner/repo", base, nil, []string{root}); err == nil || !strings.Contains(err.Error(), "multiple checkouts") {
		t.Fatalf("ambiguous result: %v", err)
	}
	got, err = ResolveRepoPath(context.Background(), "github.com", "owner/repo", base, map[string]string{"owner/repo": first}, []string{root})
	if err != nil || got != first {
		t.Fatalf("explicit ambiguity resolution: %q %v", got, err)
	}
}

func TestResolveRepoPathRejectsUnsafeAndMissingInput(t *testing.T) {
	for _, tc := range []struct {
		repo  string
		paths map[string]string
		roots []string
	}{
		{"../repo", nil, nil}, {"owner/..", nil, nil}, {"owner/repo", map[string]string{"owner/*": "relative/*"}, nil},
		{"owner/repo", map[string]string{"owner/*": "/tmp/no-wildcard"}, nil}, {"owner/repo", map[string]string{"owner/repo": "/tmp/*"}, nil},
		{"owner/repo", nil, []string{"relative"}},
	} {
		if _, err := ResolveRepoPath(context.Background(), "github.com", tc.repo, t.TempDir(), tc.paths, tc.roots); err == nil {
			t.Fatalf("accepted invalid input: %#v", tc)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ResolveRepoPath(ctx, "github.com", "owner/repo", t.TempDir(), nil, nil); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := ResolveRepoPath(ctx, "github.com", "owner/repo", "", map[string]string{"owner/repo": filepath.Join(t.TempDir(), "missing")}, nil); err == nil {
		t.Fatal("ignored cancellation for a missing mapped path")
	}
}

func TestLoadActionConfigIncludesRepoPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	data := `repoPaths:
  "glg/*": "~/github/glg/*"
  "glg/special": "/srv/special"
repoSearchRoots: ["~/Projects"]
actions:
  - key: ctrl+w
    name: Work on issue
    on: issue
    run: [/tmp/agent-fix, "{{.IssueNumber}}", "{{.RepoName}}", "{{.RepoPath}}"]
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadActionConfig(path, true)
	if err != nil || len(cfg.Actions) != 1 || len(cfg.RepoPaths) != 2 || len(cfg.RepoSearchRoots) != 1 {
		t.Fatalf("config=%#v err=%v", cfg, err)
	}
}
