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
	current    map[string]github.IssueMetadataOption
	options    []github.IssueMetadataOption
	canEdit    bool
	readErr    error
	writeErr   error
	failWrite  int
	writeApply bool
	writes     []string
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
		data.Candidates = append([]github.IssueMetadataOption(nil), s.options...)
	}
	return data, nil
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
			m, _ = editorKey(m, "/")
			m, _ = editorKey(m, "B")
			m, _ = editorKey(m, "e")
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
