package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func browserTestModel() Model {
	model := NewModel(nil)
	model.screen = screenBoard
	model.host = "github.example"
	model.selectedOwner = &github.Owner{Login: "acme", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 7, Title: "Board", URL: "https://github.example/orgs/acme/projects/7"}
	model.view = &github.View{Number: 2, Name: "Roadmap", ProjectID: "project-id"}
	model.items = []github.Item{{ID: "issue-item", Content: &github.Content{Kind: "Issue", Number: 42, Title: "Fix it", URL: "https://github.example/acme/repo/issues/42"}}}
	return model
}

func TestOpenBrowserUsesSelectedIssueURL(t *testing.T) {
	model := browserTestModel()
	var opened string
	model.openBrowser = func(target string) error {
		opened = target
		return nil
	}
	updated, cmd := model.Update(keyPress("o"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("browser command was not started")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if opened != "https://github.example/acme/repo/issues/42" || model.status != "Opened in browser." {
		t.Fatalf("opened URL/status = %q / %q", opened, model.status)
	}
}

func TestOpenBrowserUsesProjectURLWhenNoCardIsSelected(t *testing.T) {
	model := browserTestModel()
	model.items = nil
	opened := ""
	model.openBrowser = func(target string) error {
		opened = target
		return nil
	}
	updated, cmd := model.Update(keyPress("o"))
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if opened != model.selectedProject.URL || model.status != "Opened in browser." {
		t.Fatalf("project URL/status = %q / %q", opened, model.status)
	}
}

func TestOpenBrowserFallsBackToConfiguredHostProjectURL(t *testing.T) {
	model := browserTestModel()
	model.items[0].Content.URL = ""
	model.selectedProject.URL = ""
	model.openBrowser = func(target string) error {
		if target != "https://github.example/orgs/acme/projects/7" {
			t.Fatalf("fallback URL = %q", target)
		}
		return nil
	}
	updated, cmd := model.Update(keyPress("o"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("project fallback browser command was not started")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if !strings.Contains(model.status, "opened the project instead") {
		t.Fatalf("fallback status = %q", model.status)
	}
}

func TestOpenBrowserReportsActionableFailureAndMissingTarget(t *testing.T) {
	model := browserTestModel()
	model.openBrowser = func(string) error { return errors.New("xdg-open unavailable") }
	updated, cmd := model.Update(keyPress("o"))
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if !strings.Contains(model.status, "Could not open browser") || !strings.Contains(model.status, "https://github.example/acme/repo/issues/42") {
		t.Fatalf("browser failure status = %q", model.status)
	}

	model = NewModel(nil)
	updated, cmd = model.Update(keyPress("o"))
	model = updated.(Model)
	if cmd != nil || !strings.Contains(model.status, "Select an issue or project") {
		t.Fatalf("missing target state = status:%q command nil:%v", model.status, cmd == nil)
	}
}

func TestDetailUsesSidePaneWideAndFullscreenNarrow(t *testing.T) {
	model := browserTestModel()
	model.height = 42
	model.detailVisible = true
	model.detail = &github.ItemDetail{ID: "issue-item", Content: &github.Content{Kind: "Issue", Number: 42, Title: "Detail title", Body: "Issue body", BodyAvailable: true}}
	model.width = 150
	if !model.wideDetailLayout() {
		t.Fatal("wide terminal did not select side-pane layout")
	}
	wide := ansi.Strip(model.View().Content)
	if !strings.Contains(wide, "Roadmap") || !strings.Contains(wide, "Detail title") {
		t.Fatalf("wide layout omitted board or details: %q", wide)
	}
	for _, line := range strings.Split(wide, "\n") {
		if lipgloss.Width(line) > model.frameContentWidth()+4 {
			t.Fatalf("wide layout exceeded content width: %d > %d", lipgloss.Width(line), model.frameContentWidth()+4)
		}
	}

	model.width = 90
	if model.wideDetailLayout() {
		t.Fatal("narrow terminal selected side-pane layout")
	}
	narrow := ansi.Strip(model.View().Content)
	if !strings.Contains(narrow, "Detail title") || strings.Contains(narrow, "All items") || strings.Contains(narrow, "Fix it") || !strings.Contains(model.footerHints(), "esc close") {
		t.Fatalf("narrow detail overlay/footer = %q / %q", narrow, model.footerHints())
	}
	model.height = 10
	if got := lipgloss.Height(model.renderDetailPanel()); got > model.bodyCanvasHeight()+4 {
		t.Fatalf("short-height detail frame too tall: %d", got)
	}
}
