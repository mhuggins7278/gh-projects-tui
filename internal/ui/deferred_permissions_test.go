package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type deniedIssuePermissionSource struct {
	issueMembershipSource
	checks, readbacks int
}

func (s *deniedIssuePermissionSource) SetIssueClosed(context.Context, string, bool) error {
	s.checks++
	return &github.MutationError{Err: errors.New("you do not have permission to change this issue")}
}

func (s *deniedIssuePermissionSource) ReadIssueState(ctx context.Context, id string) (github.IssueState, error) {
	s.readbacks++
	return s.issueMembershipSource.ReadIssueState(ctx, id)
}

func TestDeferredIssuePermissionDenialRollsBackWithoutReadback(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		for _, state := range []string{"OPEN", "CLOSED"} {
			t.Run(string(layout)+"/"+state, func(t *testing.T) {
				source := &deniedIssuePermissionSource{}
				m := tableActionModel(source)
				view := writableStatusView()
				view.Layout = layout
				m.view = &view
				m.items = []github.Item{{ID: "one", Content: &github.Content{ID: "issue", Kind: "Issue", State: state}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
				m, _ = editorKey(m, "x")
				if !m.issueActionConfirm {
					t.Fatal("unknown eager permission incorrectly disabled the action")
				}
				m, cmd := editorKey(m, "enter")
				if cmd == nil || m.items[0].Content.State == state {
					t.Fatal("confirmed action did not project its optimistic state")
				}
				m = runMembershipReads(t, m, cmd)
				if source.checks != 1 || source.writes != 0 || source.readbacks != 0 ||
					m.items[0].Content.State != state || m.issueActionPending != nil || m.mutationLoading ||
					!strings.Contains(m.status, "permission") {
					t.Fatalf("preflight denial did not roll back cleanly: %s", m.status)
				}
			})
		}
	}
}

func TestSelectedFieldEditorLoadsPermissionMissingFromRows(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		t.Run(string(layout), func(t *testing.T) {
			field := github.Field{ID: "project-field", Name: "Org field", DataType: "TEXT", IsIssueField: true, IssueFieldID: "issue-field"}
			m, source := editorModel(field)
			m.view.Layout = layout
			denied := false
			content := *source.canonical[0].Content
			content.ViewerCanSetFields = &denied
			source.canonical[0].Content = &content
			if m.items[0].Content.ViewerCanSetFields != nil {
				t.Fatal("fixture must start without eager row permissions")
			}
			m, _ = editorKey(m, "e")
			if m.tableEditor != nil || len(source.writes) != 0 || len(source.clears) != 0 {
				t.Fatal("selected-item permission denial allowed issue-field editing")
			}
		})
	}
}
