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

func TestDetailOverlaysCurrentViewAtAnyWidth(t *testing.T) {
	model := browserTestModel()
	model.height = 42
	model.detailVisible = true
	model.detail = &github.ItemDetail{ID: "issue-item", Content: &github.Content{Kind: "Issue", Number: 42, Title: "Detail title", Body: "Issue body", BodyAvailable: true}}
	for _, width := range []int{150, 90} {
		model.width = width
		baseModel := model
		baseModel.detailVisible = false
		base := ansi.Strip(baseModel.View().Content)
		overlay := ansi.Strip(model.View().Content)
		if !strings.Contains(overlay, "Roadmap") || !strings.Contains(overlay, "Detail title") {
			t.Fatalf("width %d omitted current view or details: %q", width, overlay)
		}
		baseHeader, overlayHeader := "", ""
		for _, line := range strings.Split(base, "\n") {
			if strings.Contains(line, "Roadmap") {
				baseHeader = line
				break
			}
		}
		for _, line := range strings.Split(overlay, "\n") {
			if strings.Contains(line, "Roadmap") {
				overlayHeader = line
				break
			}
		}
		if baseHeader == "" || overlayHeader != baseHeader {
			t.Fatalf("width %d shifted the underlying view header: base=%q overlay=%q", width, baseHeader, overlayHeader)
		}
		for _, line := range strings.Split(overlay, "\n") {
			if lipgloss.Width(line) > model.frameContentWidth()+4 {
				t.Fatalf("width %d overlay exceeded content width: %d > %d", width, lipgloss.Width(line), model.frameContentWidth()+4)
			}
		}
	}

	model.width = 150
	model.view.Layout = github.TableLayout
	model.view.Fields = []github.Field{{Name: "Title", DataType: "TITLE"}}
	table := ansi.Strip(model.View().Content)
	if !strings.Contains(table, "Roadmap") || !strings.Contains(table, "Detail title") || !strings.Contains(table, "Fix it") {
		t.Fatalf("table detail overlay omitted table or details: %q", table)
	}
	if !strings.Contains(model.footerHints(), "esc close") {
		t.Fatalf("detail footer hints = %q", model.footerHints())
	}
	model.height = 10
	if got := lipgloss.Height(model.renderDetailPanel()); got > model.bodyCanvasHeight()+4 {
		t.Fatalf("short-height detail frame too tall: %d", got)
	}
}
