package ui

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func actionTestModel() Model {
	m, _ := editorModel(github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"})
	m.items[0].Content.ID = "issue-id"
	m.items[0].Content.Number = 42
	m.items[0].Content.Title = "A title; $(touch /tmp/should-not-run)"
	m.items[0].Content.URL = "https://example.test/owner/repo/issues/42"
	m.items[0].Content.RepositoryFullName = "owner/repo"
	m.host = "example.test"
	return m
}

func TestCustomActionBuildsSeparateArgumentsAndHost(t *testing.T) {
	m := actionTestModel()
	action := config.Action{Name: "Example", On: "issue", Run: []string{"gh", "issue", "view", "{{.IssueNumber}}", "--repo", "{{.RepoName}}", "{{.IssueTitle}}", "{{.ProjectItemID}}"}}
	command, itemID, err := m.customActionCommand(action)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gh", "issue", "view", "42", "--repo", "owner/repo", "A title; $(touch /tmp/should-not-run)", "one"}
	if !reflect.DeepEqual(command.Args, want) || itemID != "one" {
		t.Fatalf("args=%#v item=%s", command.Args, itemID)
	}
	hostCount := 0
	for _, env := range command.Env {
		if strings.HasPrefix(strings.ToUpper(env), "GH_HOST=") {
			hostCount++
			if env != "GH_HOST=example.test" {
				t.Fatalf("wrong host: %q", env)
			}
		}
	}
	if hostCount != 1 {
		t.Fatalf("GH_HOST count %d", hostCount)
	}
}

func TestCustomActionScopeAndMissingIdentity(t *testing.T) {
	m := actionTestModel()
	action := config.Action{Name: "PR", On: "pr", Run: []string{"gh", "pr", "view", "{{.PrNumber}}"}}
	if _, _, err := m.customActionCommand(action); err == nil {
		t.Fatal("PR action accepted issue")
	}
	m.items[0].Content.Kind = "PullRequest"
	cmd, _, err := m.customActionCommand(action)
	if err != nil || cmd.Args[3] != "42" {
		t.Fatalf("PR context: %#v %v", cmd, err)
	}
	m.items[0].Content.Kind = "DraftIssue"
	if _, _, err := m.customActionCommand(action); err == nil {
		t.Fatal("draft accepted")
	}
	m.items[0].Content.Kind = "Issue"
	m.items[0].Content.RepositoryFullName = ""
	if _, _, err := m.customActionCommand(config.Action{Name: "Issue", On: "issue", Run: []string{"gh"}}); err == nil {
		t.Fatal("missing repository accepted")
	}
}

func TestCustomActionUsesOpenDetailAndRejectsMissingProjectItem(t *testing.T) {
	m := actionTestModel()
	m.detailVisible = true
	m.detailItemID = "external-issue"
	m.detail = &github.ItemDetail{Content: &github.Content{ID: "external-issue", Kind: "Issue", Number: 7, RepositoryFullName: "outside/repo"}}
	cmd, _, err := m.customActionCommand(config.Action{Name: "Issue", On: "issue", Run: []string{"gh", "{{.RepoName}}", "{{.IssueNumber}}"}})
	if err != nil || !reflect.DeepEqual(cmd.Args, []string{"gh", "outside/repo", "7"}) {
		t.Fatalf("detail context: %#v %v", cmd, err)
	}
	if _, _, err := m.customActionCommand(config.Action{Name: "Issue", On: "issue", Run: []string{"gh", "{{.ProjectItemID}}"}}); err == nil {
		t.Fatal("outside issue had a project item ID")
	}
}

func TestCustomActionKeyAndRefreshLifecycle(t *testing.T) {
	m := actionTestModel()
	m.SetActions([]config.Action{{Key: "ctrl+p", Name: "View issue", On: "issue", Run: []string{"gh", "issue", "view", "{{.IssueNumber}}"}}})
	key := tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl})
	m.filtering = true
	updated, cmd := m.Update(key)
	if updated.(Model).actionRunning || cmd != nil {
		t.Fatal("action ran while searching")
	}
	m.filtering = false
	m.detailVisible, m.detailItemID = true, "one"
	m.detail = &github.ItemDetail{ID: "one", Content: m.items[0].Content}
	scope := m.currentBoardReadScope()
	updated, cmd = m.Update(key)
	m = updated.(Model)
	if cmd == nil || !m.actionRunning {
		t.Fatalf("action did not start: %s", m.status)
	}
	updated, reload := m.Update(customActionDoneMsg{scope: scope, itemID: "one", name: "View issue", err: errors.New("exit status 1")})
	m = updated.(Model)
	if reload == nil || m.actionRunning || !m.detailVisible || !m.detailLoading || m.pendingIssueReload == nil || !strings.Contains(m.status, "failed") {
		t.Fatalf("action completion: status=%s loading=%v", m.status, m.detailLoading)
	}
}

func TestCustomActionDoesNotRunDuringWritesOrEditing(t *testing.T) {
	m := actionTestModel()
	m.SetActions([]config.Action{{Key: "ctrl+p", Name: "View", On: "issue", Run: []string{"gh"}}})
	m.mutationLoading = true
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl}))
	if cmd != nil || updated.(Model).actionRunning {
		t.Fatal("action ran during a write")
	}
}

func TestCustomActionsAreVisibleAndHelpScrolls(t *testing.T) {
	m := actionTestModel()
	m.width, m.height = 80, 24
	m.SetActions([]config.Action{{Key: "ctrl+p", Name: "Pin issue", On: "issue", Run: []string{"gh"}}})
	m, _ = editorKey(m, "?")
	if !m.showHelp || !strings.Contains(m.View().Content, "Pin issue") {
		t.Fatalf("custom action missing from visible help: visible=%v view=%q", m.showHelp, m.View().Content)
	}
	if help := m.helpText(); !strings.Contains(help, "Custom actions\n  ctrl+p") ||
		!strings.Contains(help, "\nNavigation\n") || !strings.Contains(help, "\nTable\n") {
		t.Fatalf("help sections missing: %q", help)
	}
	if height := lipgloss.Height(m.View().Content); height > m.height {
		t.Fatalf("help extends past terminal: %d > %d", height, m.height)
	}
	m.height = 12
	if m.helpMaxOffset() <= 0 {
		t.Fatal("short terminal should require help scrolling")
	}
	m, _ = editorKey(m, "j")
	if m.helpOffset != 1 {
		t.Fatal("help did not scroll")
	}
	m, _ = editorKey(m, "esc")
	if m.showHelp || m.helpOffset != 0 {
		t.Fatal("help did not close")
	}
}

func TestCustomActionResolvesRepoPathBeforeLaunching(t *testing.T) {
	m := actionTestModel()
	path := filepath.Join(t.TempDir(), "repo with spaces $(literal)")
	action := config.Action{Key: "ctrl+w", Name: "Work on issue", On: "issue", Run: []string{"agent-fix", "{{.IssueNumber}}", "{{.RepoName}}", "{{.RepoPath}}"}}
	m.SetActionConfig(config.ActionsConfig{Actions: []config.Action{action}, RepoPaths: map[string]string{"owner/repo": path}})
	updated, prepare := m.Update(tea.KeyPressMsg(tea.Key{Code: 'w', Mod: tea.ModCtrl}))
	m = updated.(Model)
	if prepare == nil || !m.actionPreparing || !m.actionRunning {
		t.Fatal("path lookup did not start")
	}
	msg, ok := prepare().(customActionPreparedMsg)
	if !ok || msg.err != nil || msg.repoPath != path {
		t.Fatalf("resolution=%#v", msg)
	}
	updated, run := m.Update(msg)
	m = updated.(Model)
	if run == nil || m.actionPreparing || !m.actionRunning {
		t.Fatalf("action did not launch: %s", m.status)
	}
	msg.values["RepoPath"] = msg.repoPath
	command, err := buildCustomActionCommand(action, msg.values, m.host)
	if err != nil || !reflect.DeepEqual(command.Args, []string{"agent-fix", "42", "owner/repo", path}) {
		t.Fatalf("args=%#v err=%v", command.Args, err)
	}
}

func TestCustomActionPathLookupCancellationAndStaleSelection(t *testing.T) {
	for _, change := range []string{"cancel", "selection"} {
		t.Run(change, func(t *testing.T) {
			m := actionTestModel()
			path := filepath.Join(t.TempDir(), "missing")
			m.SetActionConfig(config.ActionsConfig{Actions: []config.Action{{Key: "ctrl+w", Name: "Agent", On: "issue", Run: []string{"agent-fix", "{{.RepoPath}}"}}}, RepoPaths: map[string]string{"owner/repo": path}})
			updated, prepare := m.Update(tea.KeyPressMsg(tea.Key{Code: 'w', Mod: tea.ModCtrl}))
			m = updated.(Model)
			msg := prepare()
			if change == "cancel" {
				m, _ = editorKey(m, "esc")
			} else {
				m.items[0].Content.ID = "different-issue"
			}
			updated, run := m.Update(msg)
			m = updated.(Model)
			if run != nil || m.actionRunning || m.actionPreparing {
				t.Fatalf("stale action was launched: %s", m.status)
			}
		})
	}
}

func TestActionNeedsRepoPathUsesTemplatesOnly(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"literal RepoPath in text", false}, {"{{.RepoPath}}", true}, {"{{ printf \"%s\" .RepoPath }}", true}, {"{{index . \"RepoPath\"}}", true}, {"{{index . (printf \"Repo%s\" \"Path\")}}", true}, {"{{index . \"RepoName\"}}", false}, {"{{.RepoName}}", false},
	} {
		if got := actionNeedsRepoPath(config.Action{Run: []string{tc.text}}); got != tc.want {
			t.Fatalf("%q: got %v", tc.text, got)
		}
	}
}

func TestCustomActionExpandsHomeInExecutableOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	command, err := buildCustomActionCommand(config.Action{Run: []string{"~/bin/agent-fix", "~/literal"}}, map[string]string{}, "github.com")
	if err != nil || !reflect.DeepEqual(command.Args, []string{filepath.Join(home, "bin", "agent-fix"), "~/literal"}) {
		t.Fatalf("args=%#v err=%v", command.Args, err)
	}
}
