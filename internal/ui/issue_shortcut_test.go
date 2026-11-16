package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestIssueStateShortcutInBoardAndTable(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		for _, state := range []string{"OPEN", "CLOSED"} {
			t.Run(string(layout)+"/"+state, func(t *testing.T) {
				source := &issueMembershipSource{closed: state == "CLOSED"}
				m := tableActionModel(source)
				view := writableStatusView()
				view.Layout = layout
				m.view = &view
				m.items = []github.Item{{ID: "one", Content: &github.Content{ID: "issue", Kind: "Issue", State: state, Number: 1}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
				m, _ = editorKey(m, "x")
				if !m.issueActionConfirm || m.detailVisible || !strings.Contains(m.footerHints(), "confirm") {
					t.Fatal("missing confirmation")
				}
				m, cmd := editorKey(m, "enter")
				wanted := "CLOSED"
				if state == "CLOSED" {
					wanted = "OPEN"
				}
				if cmd == nil || m.items[0].Content.State != wanted {
					t.Fatal("state not projected")
				}
				m = runMembershipReads(t, m, cmd)
				if source.writes != 1 || source.closed != (wanted == "CLOSED") || m.issueActionPending != nil {
					t.Fatalf("state change failed: %#v", m.issueActionPending)
				}
			})
		}
	}
}

func TestIssueStateShortcutCancelPermissionAndRollback(t *testing.T) {
	source := &issueMembershipSource{}
	m := tableActionModel(source)
	m.items[0].Content.ID, m.items[0].Content.State = "issue", "OPEN"
	m, _ = editorKey(m, "x")
	m, _ = editorKey(m, "esc")
	if m.issueActionConfirm || source.writes != 0 {
		t.Fatal("cancel failed")
	}
	denied := false
	m.items[0].Content.ViewerCanClose = &denied
	m, _ = editorKey(m, "x")
	if m.issueActionConfirm {
		t.Fatal("ignored permission")
	}
	m.items[0].Content.ViewerCanClose = nil
	m, _ = editorKey(m, "x")
	m, _ = editorKey(m, "enter")
	pending := m.issueActionPending
	updated, _ := m.Update(issueActionMsg{actionID: pending.id, itemID: "one", err: &github.MutationError{Err: errors.New("denied"), Ambiguous: false}})
	m = updated.(Model)
	if m.items[0].Content.State != "OPEN" || !strings.Contains(m.status, "denied") {
		t.Fatalf("failed to roll back: %s %s", m.items[0].Content.State, m.status)
	}
}

func TestIssueStateShortcutExpandedSubIssue(t *testing.T) {
	source := &issueMembershipSource{}
	m := tableActionModel(source)
	m.items = []github.Item{{ID: "parent", Content: &github.Content{ID: "parent", Kind: "Issue", SubIssueTotal: 1}}}
	m.tableSubIssues = map[string]*subIssueState{"parent": {expanded: true, complete: true, items: []github.Item{{ID: "child", Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN"}}}}}
	m.tableFocusID = "child"
	m, _ = editorKey(m, "x")
	m, cmd := editorKey(m, "enter")
	if cmd == nil || m.tableSubIssues["parent"].items[0].Content.State != "CLOSED" {
		t.Fatal("child state not projected")
	}
}
