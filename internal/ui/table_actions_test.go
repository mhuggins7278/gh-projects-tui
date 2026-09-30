package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type fakeTableActionSource struct {
	fakePickerSource
	archive []string
	remove  []string
	err     error
}

func (f *fakeTableActionSource) ArchiveProjectItem(_ context.Context, projectID, itemID string) error {
	f.archive = append(f.archive, projectID+":"+itemID)
	return f.err
}

func (f *fakeTableActionSource) RemoveProjectItem(_ context.Context, projectID, itemID string) error {
	f.remove = append(f.remove, projectID+":"+itemID)
	return f.err
}

func tableActionModel(source DiscoverySource) Model {
	m := newPickerModel(source)
	m.screen = screenBoard
	m.view = &github.View{ProjectID: "project", Layout: github.TableLayout, ViewerCanUpdate: true}
	m.selectedOwner = &github.Owner{Login: "owner"}
	m.selectedProject = &github.Project{Number: 1}
	m.items = []github.Item{{ID: "one", Content: &github.Content{Kind: "Issue", Number: 1, Title: "One"}}, {ID: "two", Content: &github.Content{Kind: "PullRequest", Number: 2, Title: "Two"}}}
	return m
}

func TestTableActionsConfirmSelectedItemAndRefresh(t *testing.T) {
	for _, tc := range []struct {
		key    string
		remove bool
	}{{"a", false}, {"D", true}} {
		t.Run(tc.key, func(t *testing.T) {
			source := &fakeTableActionSource{}
			m := tableActionModel(source)
			updated, _ := m.Update(keyPress("j"))
			m = updated.(Model)
			updated, cmd := m.Update(keyPress(tc.key))
			m = updated.(Model)
			if cmd != nil || m.tableAction == nil || m.tableAction.itemID != "two" || !strings.Contains(m.footerHints(), "confirm") {
				t.Fatalf("confirmation = %#v", m.tableAction)
			}
			updated, cmd = m.Update(keyPress("enter"))
			m = updated.(Model)
			if cmd == nil || m.tableAction.phase != "saving" {
				t.Fatalf("action was not submitted after confirmation")
			}
			updated, _ = m.Update(cmd())
			m = updated.(Model)
			if m.tableAction != nil || !m.itemsLoading || m.tableFocusID != "one" {
				t.Fatalf("save/readback = action:%#v loading:%v focus:%q", m.tableAction, m.itemsLoading, m.tableFocusID)
			}
			if tc.remove {
				if len(source.remove) != 1 || source.remove[0] != "project:two" || len(source.archive) != 0 {
					t.Fatalf("remove calls: %#v, archive: %#v", source.remove, source.archive)
				}
			} else if len(source.archive) != 1 || source.archive[0] != "project:two" || len(source.remove) != 0 {
				t.Fatalf("archive calls: %#v, remove: %#v", source.archive, source.remove)
			}
		})
	}
}

func TestTableActionsCancelAndBlockUncertainRetries(t *testing.T) {
	source := &fakeTableActionSource{err: &github.MutationError{Err: errors.New("connection lost"), Ambiguous: true}}
	m := tableActionModel(source)
	original := cloneItems(m.items)
	updated, _ := m.Update(keyPress("a"))
	m = updated.(Model)
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(Model)
	if m.tableAction != nil || len(source.archive) != 0 {
		t.Fatal("cancel submitted an action")
	}
	updated, _ = m.Update(keyPress("a"))
	m = updated.(Model)
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.tableAction == nil || m.tableAction.phase != "reconciling" {
		t.Fatalf("ambiguous action = %#v", m.tableAction)
	}
	updated, _ = m.Update(tableActionReadbackMsg{scope: m.tableAction.scope, readID: m.tableAction.readID, items: original})
	m = updated.(Model)
	if m.tableAction == nil || m.tableAction.phase != "blocked" {
		t.Fatalf("uncertain result was not blocked: %#v", m.tableAction)
	}
	updated, cmd = m.Update(keyPress("a"))
	m = updated.(Model)
	if cmd != nil || len(source.archive) != 1 {
		t.Fatal("duplicate action was submitted")
	}
}

func TestTableActionCompletionSurvivesReadGenerationChange(t *testing.T) {
	source := &fakeTableActionSource{}
	m := tableActionModel(source)
	updated, _ := m.Update(keyPress("a"))
	m = updated.(Model)
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(Model)
	result := cmd()
	m.generation++
	updated, next := m.Update(result)
	m = updated.(Model)
	if next == nil || m.tableAction != nil || !m.itemsLoading {
		t.Fatalf("stale completion changed view: action=%#v status=%q", m.tableAction, m.status)
	}
}

func TestTableGroupMoveAndReorderUseSelectedRow(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}, {ID: "done", Name: "Done"}}}
	items := []github.Item{
		{ID: "one", Content: &github.Content{Title: "First"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Available: true}}},
		{ID: "two", Content: &github.Content{Title: "Second"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Available: true}}},
	}
	mutations := []string{}
	source := fakePickerSource{canonicalItems: &items, mutations: &mutations}
	m := tableActionModel(source)
	m.view.GroupByFields = []github.Field{status}
	m.items = cloneItems(items)
	updated, _ := m.Update(keyPress("j"))
	m = updated.(Model)
	updated, _ = m.Update(keyPress("m"))
	m = updated.(Model)
	if m.tableMove == nil {
		t.Fatalf("group picker did not open: %q", m.status)
	}
	updated, _ = m.Update(keyPress("j"))
	m = updated.(Model)
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(Model)
	if cmd == nil || m.tableMove != nil || m.tableFocusID != "two" {
		t.Fatalf("group move = cmd:%v picker:%#v focus:%q", cmd, m.tableMove, m.tableFocusID)
	}
	m = runMutationCommands(t, m, cmd)
	if len(mutations) != 1 || mutations[0] != "field:done" || m.tableFocusID != "two" || m.mutationSession != nil {
		t.Fatalf("group move result: mutations=%#v focus=%q session=%#v status=%q", mutations, m.tableFocusID, m.mutationSession, m.status)
	}
	// Reordering remains a separate action and targets the visibly selected row.
	m = tableActionModel(source)
	m.items = cloneItems(items)
	updated, _ = m.Update(keyPress("J"))
	m = updated.(Model)
	if m.mutationSession == nil || m.mutationSession.intents[0].itemID != "one" || m.mutationSession.intents[0].kind != boardMutationReorder {
		t.Fatalf("row reorder intent = %#v, status %q", m.mutationSession, m.status)
	}
	m = runMutationCommands(t, m, m.settleMutationCmd(m.mutationSession.id))
	if len(mutations) != 2 || mutations[1] != "position:two" || m.tableFocusID != "one" || m.mutationSession != nil {
		t.Fatalf("reorder result: mutations=%#v focus=%q status=%q", mutations, m.tableFocusID, m.status)
	}
}

func TestTableCanMoveIntoEmptyNoStatusGroup(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}}}
	canonical := []github.Item{{ID: "one", Content: &github.Content{Title: "First"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Available: true}}}}
	mutations := []string{}
	m := tableActionModel(fakePickerSource{canonicalItems: &canonical, mutations: &mutations})
	m.view.GroupByFields = []github.Field{status}
	m.items = cloneItems(canonical)
	updated, _ := m.Update(keyPress("m"))
	m = updated.(Model)
	if m.tableMove == nil || len(m.tableMove.lanes) != 2 || m.tableMove.lanes[0].Key != "no-value" {
		t.Fatalf("available destinations = %#v", m.tableMove)
	}
	updated, _ = m.Update(keyPress("k"))
	m = updated.(Model)
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(Model)
	m = runMutationCommands(t, m, cmd)
	if len(mutations) != 1 || mutations[0] != "clear" || m.tableFocusID != "one" {
		t.Fatalf("unset move = mutations:%#v focus:%q status:%q", mutations, m.tableFocusID, m.status)
	}
}

func TestTableReorderRejectsSavedFieldSort(t *testing.T) {
	m := tableActionModel(fakePickerSource{})
	m.view.SortByFields = []github.SortField{{Direction: "ASC", Field: github.Field{Name: "Title", DataType: "TITLE"}}}
	updated, cmd := m.Update(keyPress("J"))
	m = updated.(Model)
	if cmd != nil || m.mutationSession != nil || !strings.Contains(m.status, "saved field sort") {
		t.Fatalf("sorted table reorder = cmd:%v session:%#v status:%q", cmd, m.mutationSession, m.status)
	}
}
