package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestTableViewRendersSavedFieldsAndRows(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{
		Name:   "Table",
		Number: 1,
		Layout: github.TableLayout,
		Fields: []github.Field{
			{Name: "Title", DataType: "TITLE"},
			{ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}}},
		},
	}
	model.items = []github.Item{
		{ID: "one", Content: &github.Content{Kind: "Issue", Title: "First item"}, FieldValues: []github.FieldValue{{FieldID: "status", Value: "Todo", Available: true}}},
		{ID: "two", Content: &github.Content{Kind: "Issue", Title: "Second item"}},
	}

	content := ansi.Strip(model.View().Content)
	for _, expected := range []string{"Table", "Title", "Status", "First item", "Todo", "Second item", "-"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("table missing %q: %s", expected, content)
		}
	}
	if !strings.Contains(content, "> First item") {
		t.Fatalf("selected table row missing: %s", content)
	}
}

func TestTableViewRemainsReadOnly(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{Layout: github.TableLayout, ViewerCanUpdate: true, ProjectID: "project"}
	model.items = []github.Item{{ID: "one", Content: &github.Content{Kind: "Issue", Title: "First item"}}}

	updated, cmd := model.Update(keyPress("J"))
	result := updated.(Model)
	if cmd != nil || result.mutationLoading || !strings.Contains(result.status, "one grouping field") {
		t.Fatalf("table mutation state = %#v, cmd nil = %v", result, cmd == nil)
	}
}
