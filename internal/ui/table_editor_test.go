package ui

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type fieldEditorSource struct {
	fakePickerSource
	field     github.Field
	canonical []github.Item
	writes    []github.ItemFieldValueUpdate
	clears    []github.ItemFieldValueClear
	err       error
	apply     bool
	reads     int
}

func (s *fieldEditorSource) PageItems(context.Context, github.Owner, int, string, string) (github.ItemsPage, error) {
	s.reads++
	return github.ItemsPage{Items: cloneItems(s.canonical)}, nil
}
func (s *fieldEditorSource) UpdateItemFieldValue(ctx context.Context, r github.ItemFieldValueUpdate) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("write missing deadline")
	}
	s.writes = append(s.writes, r)
	if s.err == nil || s.apply {
		for i := range s.canonical {
			if s.canonical[i].ID == r.ItemID {
				s.canonical[i].FieldValues = replaceFieldValue(s.canonical[i].FieldValues, s.field, r.Value)
			}
		}
	}
	return s.err
}
func (s *fieldEditorSource) ClearItemFieldValue(_ context.Context, r github.ItemFieldValueClear) error {
	s.clears = append(s.clears, r)
	if s.err == nil || s.apply {
		for i := range s.canonical {
			if s.canonical[i].ID == r.ItemID {
				s.canonical[i].FieldValues = removeFieldValue(s.canonical[i].FieldValues, s.field)
			}
		}
	}
	return s.err
}
func editorModel(field github.Field) (Model, *fieldEditorSource) {
	item := github.Item{ID: "one", Content: &github.Content{ID: "issue", Kind: "Issue", Title: "One", State: "OPEN"}, FieldValues: []github.FieldValue{{FieldID: field.ID, Value: "old", Available: true}}}
	s := &fieldEditorSource{field: field, canonical: []github.Item{item}}
	m := tableActionModel(s)
	m.items = cloneItems(s.canonical)
	m.view.Fields = []github.Field{field}
	m.width, m.height = 100, 30
	return m, s
}
func editorKey(m Model, key string) (Model, tea.Cmd) {
	msg := keyPress(key)
	if key == "ctrl+u" {
		msg = tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl})
	}
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func TestTableFieldEditorTextAndPasteDoNotTriggerShortcuts(t *testing.T) {
	m, s := editorModel(github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"})
	m, _ = editorKey(m, "e")
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "ctrl+u")
	for _, key := range []string{"q", "?", "r", "A", "v"} {
		m, _ = editorKey(m, key)
	}
	updated, _ := m.Update(tea.PasteMsg{Content: "\n pasted"})
	m = updated.(Model)
	if m.tableEditor == nil || m.tableEditor.draft != "q?rAv  pasted" || m.showHelp || m.filtering {
		t.Fatalf("editor state %#v", m.tableEditor)
	}
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if len(s.writes) != 1 || s.writes[0].Value.Text == nil || *s.writes[0].Value.Text != "q?rAv  pasted" || m.tableEditor != nil || m.mutationSession != nil {
		t.Fatalf("writes=%#v status=%s", s.writes, m.status)
	}
}

func TestTableFieldEditorClearValidationAndCancellation(t *testing.T) {
	for _, kind := range []string{"TEXT", "NUMBER", "DATE"} {
		t.Run(kind, func(t *testing.T) {
			m, s := editorModel(github.Field{ID: "field", Name: "Field", DataType: kind})
			m, _ = editorKey(m, "e")
			m, _ = editorKey(m, "enter")
			m, _ = editorKey(m, "ctrl+u")
			m, cmd := editorKey(m, "enter")
			m = runMembershipReads(t, m, cmd)
			if len(s.clears) != 1 || len(s.writes) != 0 || len(s.canonical[0].FieldValues) != 0 {
				t.Fatalf("clear=%#v", s.clears)
			}
		})
	}
	for _, tc := range []struct {
		kind, text string
		valid      bool
	}{{"NUMBER", "NaN", false}, {"NUMBER", "+Inf", false}, {"NUMBER", "bad", false}, {"NUMBER", "-1.25", true}, {"DATE", "2026-02-30", false}, {"DATE", "2026-02-28", true}} {
		editor := tableFieldEditor{fields: []github.Field{{DataType: tc.kind}}, draft: tc.text}
		value, _, err := editor.value()
		if (err == nil) != tc.valid {
			t.Fatalf("%s %s: %v", tc.kind, tc.text, err)
		}
		if value.Number != nil && math.IsNaN(*value.Number) {
			t.Fatal("NaN escaped validation")
		}
	}
	m, s := editorModel(github.Field{ID: "n", DataType: "TEXT"})
	m, _ = editorKey(m, "e")
	m, _ = editorKey(m, "esc")
	if m.tableEditor != nil || len(s.writes) != 0 {
		t.Fatal("cancel wrote a field")
	}
	m.view.ViewerCanUpdate = false
	m, _ = editorKey(m, "e")
	if m.tableEditor != nil {
		t.Fatal("readonly edit allowed")
	}
}

func TestTableSelectEditorsSaveOptionIdentities(t *testing.T) {
	for _, kind := range []string{"SINGLE_SELECT", "MULTI_SELECT", "ITERATION"} {
		t.Run(kind, func(t *testing.T) {
			field := github.Field{ID: "field", Name: "Field", DataType: kind, Options: []github.FieldOption{{ID: "a", Name: "A, B"}, {ID: "b", Name: "B"}}, Iterations: []github.Iteration{{ID: "a", Title: "Current"}, {ID: "b", Title: "Next"}}}
			m, s := editorModel(field)
			m, _ = editorKey(m, "e")
			m, _ = editorKey(m, "enter")
			m, _ = editorKey(m, "j")
			if kind == "MULTI_SELECT" {
				m, _ = editorKey(m, "space")
				m, _ = editorKey(m, "j")
				m, _ = editorKey(m, "space")
			}
			m, cmd := editorKey(m, "enter")
			m = runMembershipReads(t, m, cmd)
			if len(s.writes) != 1 || s.writes[0].Value.Text != nil {
				t.Fatalf("wrong value %#v", s.writes)
			}
			value := s.writes[0].Value
			switch kind {
			case "SINGLE_SELECT":
				if value.SingleSelectOptionID == nil || *value.SingleSelectOptionID != "a" {
					t.Fatal(value)
				}
			case "ITERATION":
				if value.IterationID == nil || *value.IterationID != "a" {
					t.Fatal(value)
				}
			case "MULTI_SELECT":
				if !reflect.DeepEqual(value.MultiSelectOptionIDs, []string{"a", "b"}) {
					t.Fatal(value)
				}
			}
		})
	}
}

func TestFieldEditorUncertainWritesReadBackWithoutResubmitting(t *testing.T) {
	for _, applied := range []bool{false, true} {
		m, s := editorModel(github.Field{ID: "notes", Name: "Notes", DataType: "TEXT", IsIssueField: true, IssueFieldID: "issue-field"})
		s.err = &github.MutationError{Err: errors.New("timeout"), Ambiguous: true}
		s.apply = applied
		m, _ = editorKey(m, "e")
		m, _ = editorKey(m, "enter")
		m, _ = editorKey(m, "ctrl+u")
		m, _ = editorKey(m, "n")
		m, cmd := editorKey(m, "enter")
		m = runMembershipReads(t, m, cmd)
		if len(s.writes) != 1 || s.writes[0].IssueID != "issue" || s.writes[0].IssueFieldID != "issue-field" {
			t.Fatal(s.writes)
		}
		if applied {
			if m.mutationSession != nil {
				t.Fatal("applied write not reconciled")
			}
			continue
		}
		if m.mutationSession == nil || !m.mutationSession.blocked {
			t.Fatal("unknown save was not blocked")
		}
		m, _ = editorKey(m, "e")
		if m.tableEditor != nil {
			t.Fatal("editor bypassed write gate")
		}
		m, cmd = editorKey(m, "r")
		m = runMembershipReads(t, m, cmd)
		if len(s.writes) != 1 || !strings.Contains(m.status, "check again") {
			t.Fatalf("write retried: %d %s", len(s.writes), m.status)
		}
	}
}

func TestTableIssueActionsAreAvailableInDetails(t *testing.T) {
	source := &issueMembershipSource{}
	m := tableActionModel(source)
	m.detailVisible = true
	m.detailItemID = "one"
	m.detail = &github.ItemDetail{Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN"}}
	m, _ = editorKey(m, "c")
	if !m.commentEditing {
		t.Fatal("table comment disabled")
	}
	m, _ = editorKey(m, "esc")
	m, _ = editorKey(m, "x")
	if !m.issueActionConfirm || !strings.Contains(m.footerHints(), "confirm") {
		t.Fatal("table close disabled")
	}
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if source.writes != 1 || m.detail.Content.State != "CLOSED" || m.pendingIssueReload == nil {
		t.Fatal("table close did not schedule filtered reload")
	}
}

func TestIssueFieldPermissionsAndInapplicableRows(t *testing.T) {
	field := github.Field{ID: "project-field", Name: "Org field", DataType: "TEXT", IsIssueField: true, IssueFieldID: "issue-field"}
	m, _ := editorModel(field)
	denied := false
	m.items[0].Content.ViewerCanSetFields = &denied
	m, _ = editorKey(m, "e")
	if m.tableEditor != nil {
		t.Fatal("issue field permission ignored")
	}
	m.items[0].Content.ViewerCanSetFields = nil
	m.items[0].Content.Kind = "PullRequest"
	m, _ = editorKey(m, "e")
	if m.tableEditor != nil {
		t.Fatal("issue field offered on PR")
	}
	m.items[0].Content.Kind = "Issue"
	m.items[0].OutsideProject = true
	m, _ = editorKey(m, "e")
	if m.tableEditor != nil {
		t.Fatal("outside-project row offered project edit")
	}
	m.items[0].OutsideProject = false
	m.detailVisible = true
	m.detailItemID = "one"
	m.detail = &github.ItemDetail{Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN", ViewerCanClose: &denied}}
	m, _ = editorKey(m, "x")
	if m.issueActionConfirm {
		t.Fatal("issue close permission ignored")
	}
}

func TestNestedIssueFieldEditRetainsIssueIdentityThroughCanonicalRead(t *testing.T) {
	field := github.Field{ID: "f", Name: "Notes", DataType: "TEXT", IsIssueField: true, IssueFieldID: "issue-field"}
	m, s := editorModel(field)
	child := s.canonical[0]
	parent := github.Item{ID: "parent", Content: &github.Content{ID: "parent-issue", Kind: "Issue", Title: "Parent", SubIssueTotal: 1}}
	s.canonical = []github.Item{parent, child}
	m.items = []github.Item{parent}
	m.tableSubIssues = map[string]*subIssueState{"parent": {expanded: true, complete: true, items: []github.Item{child}}}
	m.tableRow = 1
	m.tableFocusID = "one"
	m, _ = editorKey(m, "e")
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "ctrl+u")
	m, _ = editorKey(m, "n")
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if len(s.writes) != 1 || s.writes[0].IssueID != "issue" || s.writes[0].ItemID != "one" {
		t.Fatalf("nested routing: %#v %s", s.writes, m.status)
	}
}

func TestFieldEditRemovedItemAndStaleScopeCannotSubmit(t *testing.T) {
	field := github.Field{ID: "f", Name: "Notes", DataType: "TEXT"}
	m, s := editorModel(field)
	m, _ = editorKey(m, "e")
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "ctrl+u")
	m, _ = editorKey(m, "n")
	s.canonical = nil
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if len(s.writes) != 0 || m.mutationSession != nil {
		t.Fatal("removed item was written")
	}
	m, s = editorModel(field)
	m, _ = editorKey(m, "e")
	m, _ = editorKey(m, "enter")
	m.generation++
	m, cmd = editorKey(m, "enter")
	if cmd != nil || m.tableEditor != nil || len(s.writes) != 0 {
		t.Fatal("stale editor wrote to a changed view")
	}
}
