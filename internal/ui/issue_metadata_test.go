package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type metadataTestSource struct {
	*fieldEditorSource
	current        map[string]github.IssueMetadataOption
	options        []github.IssueMetadataOption
	canEdit        bool
	readErr        error
	writeErr       error
	failWrite      int
	writeApply     bool
	writes         []string
	candidateReads int
	searches       []string
	pageSize       int
}

func (s *metadataTestSource) ReadIssueMetadata(_ context.Context, _ string, _ github.IssueMetadataKind, candidates bool) (github.IssueMetadata, error) {
	if s.readErr != nil {
		return github.IssueMetadata{}, s.readErr
	}
	data := github.IssueMetadata{CanEdit: s.canEdit}
	for _, option := range s.current {
		data.Current = append(data.Current, option)
	}
	if candidates {
		s.candidateReads++
		data.Candidates = append([]github.IssueMetadataOption(nil), s.options...)
	}
	return data, nil
}
func (s *metadataTestSource) SearchIssueAssignees(_ context.Context, _ string, query, after string) (github.AssigneeSearchPage, error) {
	s.searches = append(s.searches, query)
	page := github.AssigneeSearchPage{CanEdit: s.canEdit}
	for _, option := range s.options {
		if strings.Contains(strings.ToLower(option.Name), strings.ToLower(query)) {
			page.Options = append(page.Options, option)
		}
	}
	if s.pageSize > 0 && len(page.Options) > s.pageSize {
		if after == "" {
			page.Options = page.Options[:s.pageSize]
			page.HasNext, page.EndCursor = true, "next"
		} else {
			page.Options = page.Options[s.pageSize:]
		}
	}
	return page, nil
}

func TestAssigneeSearchRetainsSelectionsAcrossQueries(t *testing.T) {
	m, s := metadataModel()
	s.options = append(s.options, github.IssueMetadataOption{ID: "c", Name: "Charlie"})
	m = openMetadata(t, m, "a")
	if s.candidateReads != 0 || !reflect.DeepEqual(s.searches, []string{""}) || !m.issueMetadataEditor.searching {
		t.Fatalf("unexpected initial load: candidates=%d searches=%#v", s.candidateReads, s.searches)
	}
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "j")
	m, _ = editorKey(m, "space") // Beta
	m, _ = editorKey(m, "/")
	m, search := editorKey(m, "C")
	m = runMembershipReads(t, m, search)
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "space") // Charlie
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if !reflect.DeepEqual(s.writes, []string{"add:b,c"}) || s.current["a"].ID != "a" {
		t.Fatalf("hidden selection lost: writes=%#v current=%#v", s.writes, s.current)
	}
}

func TestMetadataSelectionAppearsBeforeWrite(t *testing.T) {
	m, _ := metadataModel()
	m.detail.Fields = []github.DetailField{{Field: github.Field{ID: "labels", Name: "Labels"}, Value: &github.FieldValue{Value: "Alpha", Available: true}}}
	m = openMetadata(t, m, "l")
	m, _ = editorKey(m, "j")
	m, _ = editorKey(m, "space")
	m, cmd := editorKey(m, "enter")
	if cmd == nil || m.detail.Fields[0].Value.Value != "Alpha, Beta" {
		t.Fatalf("selection was not projected: %#v", m.detail.Fields)
	}
}

func TestMetadataEditorDoesNotDiscardPendingViewRefresh(t *testing.T) {
	for _, finish := range []string{"esc", "enter"} {
		t.Run(finish, func(t *testing.T) {
			m, source := metadataModel()
			m.view.Filter = "label:visible"
			source.canonical = nil
			refresh := m.startMutationViewRefresh()
			m = openMetadata(t, m, "l")
			updated, _ := m.Update(refresh())
			m = updated.(Model)
			if len(m.items) != 0 || m.issueMetadataEditor == nil {
				t.Fatal("pending view refresh was discarded while editor was open")
			}
			m, _ = editorKey(m, finish)
			if m.issueMetadataEditor != nil || len(m.items) != 0 {
				t.Fatal("closing editor lost refreshed membership")
			}
		})
	}
}

func TestAssigneeToggleKeepsCursorOnUser(t *testing.T) {
	m, s := metadataModel()
	s.options = append(s.options, github.IssueMetadataOption{ID: "c", Name: "Charlie"})
	m = openMetadata(t, m, "a")
	for _, key := range []string{"enter", "j", "j", "space"} {
		m, _ = editorKey(m, key)
	}
	editor := m.issueMetadataEditor
	if got := editor.visibleOptions()[editor.index].ID; got != "c" {
		t.Fatalf("cursor moved to %q after selecting Charlie", got)
	}
	m, _ = editorKey(m, "space")
	if got := editor.visibleOptions()[editor.index].ID; got != "c" {
		t.Fatalf("cursor moved to %q after deselecting Charlie", got)
	}
	m, cmd := editorKey(m, "enter")
	if cmd != nil || m.status != "No changes" {
		t.Fatalf("toggling Charlie twice should leave no changes, got %q", m.status)
	}
}

func TestAssigneeSearchIgnoresStaleResponses(t *testing.T) {
	m, _ := metadataModel()
	m = openMetadata(t, m, "a")
	editor := m.issueMetadataEditor
	oldRevision := editor.searchRevision
	m, _ = editorKey(m, "z")
	updated, _ := m.Update(assigneeSearchMsg{editorID: editor.id, revision: oldRevision, page: github.AssigneeSearchPage{CanEdit: true, Options: []github.IssueMetadataOption{{ID: "stale", Name: "stale"}}}})
	m = updated.(Model)
	if len(m.issueMetadataEditor.options) != 0 || !m.issueMetadataEditor.searchLoading {
		t.Fatal("stale results replaced newer search")
	}
}

func TestAssigneeSearchLoadsNextPageAtEnd(t *testing.T) {
	m, s := metadataModel()
	s.pageSize = 1
	m = openMetadata(t, m, "a")
	if len(m.issueMetadataEditor.options) != 1 || !m.issueMetadataEditor.searchHasNext {
		t.Fatal("initial page was not bounded")
	}
	m, _ = editorKey(m, "enter")
	m, next := editorKey(m, "j")
	m = runMembershipReads(t, m, next)
	if len(m.issueMetadataEditor.options) != 2 || m.issueMetadataEditor.searchHasNext || len(s.searches) != 2 {
		t.Fatalf("next page missing: %#v", m.issueMetadataEditor)
	}
}
func (s *metadataTestSource) ChangeIssueMetadata(_ context.Context, _ string, _ github.IssueMetadataKind, add bool, ids []string) error {
	verb := "remove:"
	if add {
		verb = "add:"
	}
	s.writes = append(s.writes, verb+strings.Join(ids, ","))
	err := s.writeErr
	if s.failWrite != 0 && len(s.writes) != s.failWrite {
		err = nil
	}
	if err == nil || s.writeApply {
		for _, id := range ids {
			if add {
				for _, option := range s.options {
					if option.ID == id {
						s.current[id] = option
					}
				}
			} else {
				delete(s.current, id)
			}
		}
	}
	return err
}
func (s *metadataTestSource) LoadItemDetail(_ context.Context, _ github.Owner, _ int, id string) (github.ItemDetail, error) {
	return github.ItemDetail{ID: id, Content: &github.Content{ID: "issue", Kind: "Issue"}}, nil
}
func metadataModel() (Model, *metadataTestSource) {
	m, source := editorModel(github.Field{ID: "field", Name: "Field", DataType: "TEXT"})
	m.view.Layout = github.BoardLayout
	m.view.GroupByFields = writableStatusView().GroupByFields
	s := &metadataTestSource{fieldEditorSource: source, canEdit: true, current: map[string]github.IssueMetadataOption{"a": {ID: "a", Name: "Alpha"}}, options: []github.IssueMetadataOption{{ID: "a", Name: "Alpha"}, {ID: "b", Name: "Beta"}}}
	m.source = s
	m.detailVisible, m.detailItemID = true, "one"
	m.detail = &github.ItemDetail{ID: "one", Content: m.items[0].Content}
	return m, s
}
func openMetadata(t *testing.T, m Model, key string) Model {
	t.Helper()
	m, cmd := editorKey(m, key)
	return runMembershipReads(t, m, cmd)
}
func TestIssueMetadataEditSearchAndDelta(t *testing.T) {
	for _, key := range []string{"l", "a"} {
		t.Run(key, func(t *testing.T) {
			m, s := metadataModel()
			m = openMetadata(t, m, key)
			if m.issueMetadataEditor == nil {
				t.Fatalf("editor did not open: %s", m.status)
			}
			if key == "l" {
				m, _ = editorKey(m, "/")
			}
			m, _ = editorKey(m, "B")
			m, search := editorKey(m, "e")
			if key == "a" {
				m = runMembershipReads(t, m, search)
			}
			m, _ = editorKey(m, "enter")
			if len(m.issueMetadataEditor.visibleOptions()) != 1 || m.issueMetadataEditor.visibleOptions()[0].ID != "b" {
				t.Fatal("search did not filter options")
			}
			m, _ = editorKey(m, "space")
			m, cmd := editorKey(m, "enter")
			m = runMembershipReads(t, m, cmd)
			if !reflect.DeepEqual(s.writes, []string{"add:b"}) || s.current["a"].ID != "a" || s.current["b"].ID != "b" || m.issueActionPending != nil {
				t.Fatalf("writes=%#v current=%#v status=%s", s.writes, s.current, m.status)
			}
			if m.pendingIssueReload == nil {
				t.Fatal("saved view refresh not queued")
			}
		})
	}
}
func TestIssueMetadataAmbiguousWriteBlocksUntilConfirmed(t *testing.T) {
	m, s := metadataModel()
	m = openMetadata(t, m, "l")
	m, _ = editorKey(m, "j")
	m, _ = editorKey(m, "space")
	s.writeErr = errors.New("timeout")
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if m.issueActionPending == nil || !m.issueActionPending.blocked || !m.mutationLoading || len(s.writes) != 1 {
		t.Fatalf("uncertain write did not block: %#v", m.issueActionPending)
	}
	s.current["b"] = s.options[1]
	m, retry := editorKey(m, "r")
	m = runMembershipReads(t, m, retry)
	if m.issueActionPending != nil || len(s.writes) != 1 || m.detailLoading || m.detailErr != nil {
		t.Fatalf("reconciliation failed: status=%s writes=%#v", m.status, s.writes)
	}
}
func TestIssueMetadataDeniedOrCancelledDoesNotWrite(t *testing.T) {
	m, s := metadataModel()
	s.canEdit = false
	m = openMetadata(t, m, "a")
	if m.issueMetadataEditor != nil || len(s.writes) != 0 {
		t.Fatal("permission denial was ignored")
	}
	s.canEdit = true
	m = openMetadata(t, m, "a")
	m, _ = editorKey(m, "enter")
	m, _ = editorKey(m, "space")
	m, _ = editorKey(m, "esc")
	if m.issueMetadataEditor != nil || len(s.writes) != 0 {
		t.Fatal("cancel submitted a write")
	}
}

func TestIssueMetadataPartialFailureReadsBackSuccessfulChange(t *testing.T) {
	m, s := metadataModel()
	m = openMetadata(t, m, "l")
	m, _ = editorKey(m, "space") // remove Alpha
	m, _ = editorKey(m, "j")
	m, _ = editorKey(m, "space") // add Beta
	s.writeErr = &github.MutationError{Err: errors.New("denied"), Ambiguous: false}
	s.failWrite = 2
	m, cmd := editorKey(m, "enter")
	m = runMembershipReads(t, m, cmd)
	if !reflect.DeepEqual(s.writes, []string{"add:b", "remove:a"}) || s.current["a"].ID != "a" || s.current["b"].ID != "b" || m.issueActionPending != nil || !strings.Contains(m.status, "Some changes") {
		t.Fatalf("partial failure: writes=%#v current=%#v status=%q", s.writes, s.current, m.status)
	}
}

func TestIssueMetadataReadbackFailureRemainsBlocked(t *testing.T) {
	m, s := metadataModel()
	m = openMetadata(t, m, "l")
	m, _ = editorKey(m, "j")
	m, _ = editorKey(m, "space")
	m, cmd := editorKey(m, "enter")
	msg := cmd()
	s.readErr = errors.New("read unavailable")
	updated, read := m.Update(msg)
	m = runMembershipReads(t, updated.(Model), read)
	if m.issueActionPending == nil || !m.issueActionPending.blocked || !m.mutationLoading {
		t.Fatal("failed readback unlocked writes")
	}
	s.readErr = nil
	m, retry := editorKey(m, "r")
	m = runMembershipReads(t, m, retry)
	if m.issueActionPending != nil || len(s.writes) != 1 {
		t.Fatal("readback retry did not reconcile without rewriting")
	}
}
