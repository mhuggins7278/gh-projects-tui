package ui

import (
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
	"testing"
)

func TestFieldSearchSavesOriginalOptionIdentity(t *testing.T) {
	field := github.Field{ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "a", Name: "Backlog"}, {ID: "b", Name: "In progress"}, {ID: "c", Name: "Done"}}}
	m, _ := editorModel(field)
	m, _ = editorKey(m, "e")
	m, _ = editorKey(m, "S")
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "D")
	if m.tableEditor.optionIndex != 3 {
		t.Fatalf("filtered option index = %d", m.tableEditor.optionIndex)
	}
	value, _, err := m.tableEditor.value()
	if err != nil || value.SingleSelectOptionID == nil || *value.SingleSelectOptionID != "c" {
		t.Fatalf("wrong filtered value: %#v %v", value, err)
	}
	m, _ = editorKey(m, "backspace")
	if m.tableEditor.filter != "" {
		t.Fatal("backspace did not clear search")
	}
}

func TestMetadataSearchNavigatesWithoutLeavingSearch(t *testing.T) {
	m, _ := metadataModel()
	m = openMetadata(t, m, "l")
	m, _ = editorKey(m, "B")
	if len(m.issueMetadataEditor.visibleOptions()) != 1 {
		t.Fatal("typing did not filter labels")
	}
	m, _ = editorKey(m, "space")
	if !m.issueMetadataEditor.selected["b"] {
		t.Fatal("space did not toggle while searching")
	}
	m, _ = editorKey(m, "esc")
	if m.issueMetadataEditor != nil {
		t.Fatal("escape did not dismiss editor")
	}
}
