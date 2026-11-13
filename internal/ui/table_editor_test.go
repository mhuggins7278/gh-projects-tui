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

type targetedFieldSource struct {
	*fieldEditorSource
	targetedReads int
}

func (s *targetedFieldSource) ReadMutationItemForProject(_ context.Context, projectID, itemID string, _ github.Field) (github.Item, error) {
	s.targetedReads++
	if projectID != "project" {
		return github.Item{}, errors.New("wrong project")
	}
	for _, item := range s.canonical {
		if item.ID == itemID {
			return item, nil
		}
	}
	return github.Item{}, github.ErrMutationItemUnavailable
}

func TestTargetedPreflightRemovedItemDoesNotBlockWrites(t *testing.T) {
	m, base := editorModel(github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"})
	base.canonical = nil
	m.source = &targetedFieldSource{fieldEditorSource: base}
	for _, key := range []string{"e", "enter", "ctrl+u", "n"} {
		m, _ = editorKey(m, key)
	}
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if m.mutationSession != nil || m.mutationLoading || len(base.writes) != 0 || len(m.items) != 0 {
		t.Fatalf("removed item left the editor blocked: session=%#v status=%s items=%#v", m.mutationSession, m.status, m.items)
	}
}

func TestFieldEditUsesTargetedPreflightAndKeepsFilteredRows(t *testing.T) {
	m, base := editorModel(github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"})
	m.view.Filter = "label:visible"
	base.canonical = append(base.canonical, github.Item{ID: "hidden", Content: &github.Content{Title: "Hidden"}})
	source := &targetedFieldSource{fieldEditorSource: base}
	m.source = source
	for _, key := range []string{"e", "enter", "ctrl+u", "n"} {
		m, _ = editorKey(m, key)
	}
	m, settle := editorKey(m, "enter")
	if len(m.items) != 1 || m.items[0].FieldValues[0].Value != "n" {
		t.Fatal("edited value was not projected immediately")
	}
	updated, read := m.Update(settle())
	m = updated.(Model)
	updated, write := m.Update(read())
	m = updated.(Model)
	if source.targetedReads != 1 || source.reads != 0 || len(m.items) != 1 || m.items[0].Content.Title != "One" {
		t.Fatalf("preflight reads=%d project reads=%d items=%#v", source.targetedReads, source.reads, m.items)
	}
	if write == nil {
		t.Fatal("write was not dispatched")
	}
}

func TestMutationViewRefreshKeepsRowsUntilCompleteAndRejectsStaleResult(t *testing.T) {
	m, source := editorModel(github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"})
	m.view.Filter = "label:visible"
	source.canonical = []github.Item{{ID: "new", Content: &github.Content{Title: "New"}}}
	refresh := m.startMutationViewRefresh()
	if len(m.items) != 1 || m.items[0].ID != "one" {
		t.Fatal("existing rows disappeared while refresh was pending")
	}
	message := refresh()
	updated, _ := m.Update(message)
	m = updated.(Model)
	if len(m.items) != 1 || m.items[0].ID != "new" {
		t.Fatalf("completed refresh not applied: %#v", m.items)
	}
	refresh = m.startMutationViewRefresh()
	message = refresh()
	m.mutationRefreshID++
	m.items = []github.Item{{ID: "current"}}
	updated, _ = m.Update(message)
	m = updated.(Model)
	if len(m.items) != 1 || m.items[0].ID != "current" {
		t.Fatal("stale refresh replaced current rows")
	}
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
	m.tableSubIssues = map[string]*subIssueState{"parent": {expanded: true, complete: true, items: cloneItems([]github.Item{child})}}
	m.tableRow = 1
	m.tableFocusID = "one"
	m, _ = editorKey(m, "e")
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "ctrl+u")
	m, _ = editorKey(m, "n")
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if got := m.tableSubIssues["parent"].items[0].FieldValues[0].Value; got != "n" {
		t.Fatalf("nested row field = %q, want saved value", got)
	}
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

func TestDetailFieldEditorUsesAllProjectFieldsAndDetailValue(t *testing.T) {
	field := github.Field{ID: "hidden", Name: "Hidden field", DataType: "TEXT"}
	m, _ := editorModel(field)
	m.view.Fields = nil
	m.detailVisible = true
	m.detailItemID = "one"
	m.detail = &github.ItemDetail{Content: m.items[0].Content, Fields: []github.DetailField{{Field: field, Value: &github.FieldValue{FieldID: field.ID, Value: "current detail", Available: true}}}}
	m, _ = editorKey(m, "e")
	if m.tableEditor == nil || !m.tableEditor.detail || len(m.tableEditor.fields) != 1 {
		t.Fatalf("detail field editor not opened: %#v", m.tableEditor)
	}
	m, _ = editorKey(m, "enter")
	if m.tableEditor.draft != "current detail" {
		t.Fatalf("got draft %q", m.tableEditor.draft)
	}
	m, _ = editorKey(m, "esc")
	m.detail.Fields[0].Value.Available = false
	m, _ = editorKey(m, "e")
	if m.tableEditor != nil {
		t.Fatal("unavailable field must not be editable")
	}
}

func TestDetailFieldEditorRespectsProjectAndIssuePermissions(t *testing.T) {
	field := github.Field{ID: "f", Name: "Field", DataType: "TEXT", IsIssueField: true, IssueFieldID: "issue-field"}
	m, _ := editorModel(field)
	m.detailVisible, m.detailItemID = true, "one"
	m.detail = &github.ItemDetail{Content: m.items[0].Content, Fields: []github.DetailField{{Field: field}}}
	denied := false
	m.detail.Content.ViewerCanSetFields = &denied
	m, _ = editorKey(m, "e")
	if m.tableEditor != nil {
		t.Fatal("issue field permission was ignored")
	}
	m.detail.Content.ViewerCanSetFields = nil
	m.view.ViewerCanUpdate = false
	m, _ = editorKey(m, "e")
	if m.tableEditor != nil {
		t.Fatal("read-only project was ignored")
	}
}

type refreshedDetailSource struct {
	*fieldEditorSource
	detail github.ItemDetail
	err    error
}

type detailFieldActionSource struct{ *refreshedDetailSource }

func (*detailFieldActionSource) IssueComment(context.Context, string, string) error { return nil }
func (*detailFieldActionSource) SetIssueClosed(context.Context, string, bool) error { return nil }

func (s *refreshedDetailSource) LoadItemDetail(context.Context, github.Owner, int, string) (github.ItemDetail, error) {
	return s.detail, s.err
}

func TestDetailFieldRefreshAfterSaveAndReconciliation(t *testing.T) {
	for _, reconciled := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "reconciled"}[reconciled], func(t *testing.T) {
			field := github.Field{ID: "f", Name: "Notes", DataType: "TEXT"}
			m, base := editorModel(field)
			m.view.Layout = github.BoardLayout
			m.view.GroupByFields = writableStatusView().GroupByFields
			newValue := "new"
			updatedItem := cloneItems(m.items)[0]
			updatedItem.FieldValues[0].Value = newValue
			source := &refreshedDetailSource{fieldEditorSource: base, detail: github.ItemDetail{ID: "one", Content: updatedItem.Content, Fields: []github.DetailField{{Field: field, Value: &updatedItem.FieldValues[0]}}}}
			m.source = source
			m.detailVisible, m.detailItemID = true, "one"
			m.detail = &github.ItemDetail{ID: "one", Content: m.items[0].Content, Fields: []github.DetailField{{Field: field, Value: &m.items[0].FieldValues[0]}}}
			intent := boardMutationIntent{kind: boardMutationSetField, itemID: "one", field: field, value: github.FieldValueInput{Text: &newValue}, description: "Edit Notes"}
			plan := boardMutationPlan{intent: intent, projectID: m.view.ProjectID, field: field, fieldValue: intent.value, hasField: true, beforeItems: cloneItems(m.items)}
			m.mutationSession = &boardMutationSession{id: 1, writeID: 1, readID: 1, generation: m.generation, owner: *m.selectedOwner, projectNumber: m.selectedProject.Number, projectID: m.view.ProjectID, view: *m.view, intents: []boardMutationIntent{intent}, plan: &plan, fieldEdited: true}
			var cmd tea.Cmd
			if reconciled {
				m.mutationSession.attempt = &mutationAttemptResult{err: errors.New("timeout"), ambiguous: true}
				updated, next := m.updateMutationReadback(mutationReadbackMsg{sessionID: 1, readID: 1, items: []github.Item{updatedItem}})
				m, cmd = updated.(Model), next
			} else {
				updated, next := m.updateBoardMutation(boardMutationMsg{sessionID: 1, writeID: 1})
				m, cmd = updated.(Model), next
			}
			if !m.detailLoading || cmd == nil {
				t.Fatal("save did not start a detail refresh")
			}
			// A table refresh also blocks edits; isolate the detail loading guard.
			m.itemsLoading = false
			m, _ = editorKey(m, "e")
			if m.tableEditor != nil {
				t.Fatal("stale detail was editable during refresh")
			}
			m = runMembershipReads(t, m, cmd)
			if m.detailLoading || m.detailErr != nil || m.detail.Fields[0].Value.Value != newValue {
				t.Fatalf("detail not refreshed: loading=%v err=%v detail=%#v", m.detailLoading, m.detailErr, m.detail)
			}
			m.itemsLoading = false
			m, _ = editorKey(m, "e")
			m, _ = editorKey(m, "enter")
			if m.tableEditor == nil || m.tableEditor.draft != newValue {
				t.Fatal("editor did not use refreshed value")
			}
		})
	}
}

func TestDetailFieldEditBlockedWhenRefreshFails(t *testing.T) {
	field := github.Field{ID: "f", Name: "Notes", DataType: "TEXT"}
	m, _ := editorModel(field)
	m.detailVisible, m.detailItemID = true, "one"
	m.detail = &github.ItemDetail{Content: m.items[0].Content, Fields: []github.DetailField{{Field: field}}}
	m.detailErr = errors.New("refresh failed")
	m, _ = editorKey(m, "e")
	if m.tableEditor != nil {
		t.Fatal("stale detail was editable after failed refresh")
	}
}

// Field edits must keep the issue detail in control until the user dismisses it,
// even when the saved view needs a fresh membership read after the write.
func TestDetailFieldSaveKeepsPanelAndFocusUntilDismissed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		layout    github.ViewLayout
		filter    string
		ambiguous bool
	}{
		{"table", github.TableLayout, "", false},
		{"filtered board", github.BoardLayout, "is:open", false},
		{"board", github.BoardLayout, "", false},
		{"table reconciled", github.TableLayout, "", true},
		{"filtered board reconciled", github.BoardLayout, "is:open", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field := github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"}
			m, base := editorModel(field)
			if tc.ambiguous {
				base.err = &github.MutationError{Err: errors.New("timeout"), Ambiguous: true}
				base.apply = true
			}
			m.view.Layout, m.view.Filter = tc.layout, tc.filter
			if tc.layout == github.BoardLayout {
				m.view.GroupByFields = writableStatusView().GroupByFields
			}
			newFieldValue := github.FieldValue{FieldID: field.ID, Value: "new", Available: true}
			source := &detailFieldActionSource{&refreshedDetailSource{fieldEditorSource: base, detail: github.ItemDetail{ID: "one", Content: m.items[0].Content, Fields: []github.DetailField{{Field: field, Value: &newFieldValue}}}}}
			m.source = source
			m.detailVisible, m.detailItemID = true, "one"
			m.detail = &github.ItemDetail{ID: "one", Content: m.items[0].Content, Fields: []github.DetailField{{Field: field, Value: &m.items[0].FieldValues[0]}}}
			m, _ = editorKey(m, "e")
			m, _ = editorKey(m, "enter")
			m, _ = editorKey(m, "ctrl+u")
			m, _ = editorKey(m, "n")
			m, _ = editorKey(m, "e")
			m, _ = editorKey(m, "w")
			m, cmd := editorKey(m, "enter")
			m = runMembershipReads(t, m, cmd)
			if !m.detailVisible || m.detail == nil || m.detailItemID != "one" || m.detailLoading || m.detail.Fields[0].Value.Value != "new" {
				t.Fatalf("detail lost or stale: visible=%v loading=%v item=%q status=%s", m.detailVisible, m.detailLoading, m.detailItemID, m.status)
			}
			if m.mutationSession != nil || m.mutationLoading {
				t.Fatal("save remained locked after completion")
			}
			m, _ = editorKey(m, "x")
			if !m.issueActionConfirm {
				t.Fatal("issue close action lost focus after saving a field")
			}
			m, _ = editorKey(m, "esc") // Cancel the close confirmation.
			m, closeCmd := editorKey(m, "esc")
			if m.detailVisible {
				t.Fatal("Escape did not close detail")
			}
			if tc.layout == github.TableLayout || tc.filter != "" {
				if closeCmd == nil {
					t.Fatal("membership refresh was not deferred until close")
				}
				m = runMembershipReads(t, m, closeCmd)
				if selected, ok := m.selectedItem(); !ok || selected.ID != "one" {
					t.Fatalf("selection lost after reload: %#v", selected)
				}
			}
		})
	}
}

func TestDetailCanCloseWhileFieldSaveIsPending(t *testing.T) {
	field := github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"}
	m, _ := editorModel(field)
	m.detailVisible, m.detailItemID = true, "one"
	m.detail = &github.ItemDetail{ID: "one", Content: m.items[0].Content}
	m.mutationLoading = true
	m.mutationSession = &boardMutationSession{}
	m, _ = editorKey(m, "esc")
	if m.detailVisible || m.mutationSession == nil {
		t.Fatal("Escape must dismiss detail without cancelling the save")
	}
}

func TestClosingDetailDuringSaveRetainsBoardSelectionAfterRefresh(t *testing.T) {
	field := github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"}
	m, base := editorModel(field)
	m.view.Layout, m.view.Filter = github.BoardLayout, "is:open"
	m.view.GroupByFields = writableStatusView().GroupByFields
	other := github.Item{ID: "other", Content: &github.Content{ID: "other-issue", Kind: "Issue", State: "OPEN"}}
	m.items = append([]github.Item{other}, m.items...)
	base.canonical = cloneItems(m.items)
	m.boardFocusID, m.boardCard = "one", 1
	m.detailVisible, m.detailItemID = true, "one"
	m.detail = &github.ItemDetail{ID: "one", Content: m.items[1].Content, Fields: []github.DetailField{{Field: field, Value: &m.items[1].FieldValues[0]}}}
	m.source = &refreshedDetailSource{fieldEditorSource: base}
	m, _ = editorKey(m, "e")
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "ctrl+u")
	m, _ = editorKey(m, "n")
	m, settle := editorKey(m, "enter")
	if settle == nil {
		t.Fatal("save was not queued")
	}
	updated, read := m.Update(settle())
	m = updated.(Model)
	if !m.mutationLoading {
		t.Fatal("save did not start")
	}
	m, _ = editorKey(m, "esc")
	if m.detailVisible {
		t.Fatal("could not close detail during save")
	}
	m = runMembershipReads(t, m, read)
	item, ok := m.selectedItem()
	if !ok || item.ID != "one" || m.mutationSession != nil || len(base.writes) != 1 {
		t.Fatalf("lost focus or save: selected=%#v writes=%d status=%s", item, len(base.writes), m.status)
	}
}
