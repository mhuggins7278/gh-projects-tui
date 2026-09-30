package ui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestTextEditorsOwnPrintableShortcuts(t *testing.T) {
	for _, mode := range []string{"picker", "board", "comment"} {
		t.Run(mode, func(t *testing.T) {
			m := NewModel(nil)
			m.debug = true
			m.screen = screenBoard
			m.filtering = mode != "comment"
			if mode == "picker" {
				m.screen = screenProjectPicker
			}
			if mode == "comment" {
				m.detailVisible, m.commentEditing = true, true
			}
			for _, text := range []string{"q", "?", "A", " ", "界"} {
				updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: []rune(text)[0], Text: text}))
				m = updated.(Model)
				if cmd != nil || m.showHelp || m.showAPI {
					t.Fatalf("editor dispatched shortcut %q", text)
				}
			}
			actual := m.filter
			if mode == "comment" {
				actual = m.commentDraft
			}
			if actual != "q?A 界" {
				t.Fatalf("text = %q", actual)
			}
			updated, _ := m.Update(keyPress("backspace"))
			m = updated.(Model)
			actual = m.filter
			if mode == "comment" {
				actual = m.commentDraft
			}
			if actual != "q?A " || !utf8.ValidString(actual) {
				t.Fatalf("backspace = %q", actual)
			}
			_, cmd := m.Update(keyPress("ctrl+c"))
			if cmd == nil {
				t.Fatal("ctrl+c did not quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("ctrl+c returned another command")
			}
		})
	}
}

func TestPasteNewlinesAreSingleLineSearchAndMultilineComment(t *testing.T) {
	m := NewModel(nil)
	m.filtering = true
	updated, _ := m.Update(tea.PasteMsg{Content: "one\r\ntwo\nthree"})
	m = updated.(Model)
	if m.filter != "one two three" {
		t.Fatalf("search = %q", m.filter)
	}
	m.filtering, m.commentEditing = false, true
	updated, _ = m.Update(tea.PasteMsg{Content: "one\ntwo"})
	m = updated.(Model)
	if m.commentDraft != "one\ntwo" {
		t.Fatalf("comment = %q", m.commentDraft)
	}
}

func TestPickerRejectsSameGenerationDifferentDestination(t *testing.T) {
	m := NewModel(nil)
	m.screen = screenViewPicker
	m.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	m.selectedProject = &github.Project{Number: 1}
	old := m.currentPickerReadScope()
	m.selectedProject = &github.Project{Number: 2}
	for _, msg := range []tea.Msg{
		viewsMsg{scope: old, generation: m.generation, views: []github.ViewSummary{{Number: 99}}},
		viewDetailMsg{scope: old, generation: m.generation, view: &github.View{Number: 99, Layout: github.BoardLayout}},
	} {
		updated, cmd := m.Update(msg)
		m = updated.(Model)
		if cmd != nil || len(m.views) != 0 || m.view != nil || m.screen != screenViewPicker {
			t.Fatal("old destination installed")
		}
	}
}

func TestPickerNewSelectionSupersedesPreviousOpen(t *testing.T) {
	source := fakePickerSource{view: github.View{Number: 1, Layout: github.BoardLayout}}
	m := NewModel(source)
	m.screen = screenViewPicker
	m.selectedOwner = &github.Owner{Login: "org"}
	m.selectedProject = &github.Project{Number: 1}
	m.views = []github.ViewSummary{{Number: 1, Layout: github.BoardLayout}, {Number: 2, Layout: github.BoardLayout}}
	updated, first := m.Update(keyPress("enter"))
	m = updated.(Model)
	m.cursor = 1
	updated, second := m.Update(keyPress("enter"))
	m = updated.(Model)
	if second == nil {
		t.Fatal("second selection did not start")
	}
	updated, cmd := m.Update(first())
	m = updated.(Model)
	if cmd != nil || m.view != nil || !m.loadingDetail {
		t.Fatal("old request replaced newer selection")
	}
}

func submittedReviewSave(t *testing.T) (Model, tea.Cmd, *[]github.Item) {
	t.Helper()
	view := writableStatusView()
	view.Layout = github.BoardLayout
	canonical := []github.Item{{ID: "one", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
	m := mutationModel(fakePickerSource{canonicalItems: &canonical}, view, canonical)
	updated, settle := m.Update(keyPress("L"))
	m = updated.(Model)
	updated, read := m.Update(settle())
	m = updated.(Model)
	updated, save := m.Update(read())
	m = updated.(Model)
	if save == nil {
		t.Fatal("no submitted save")
	}
	return m, save, &canonical
}

func TestSaveUsesOwnBaselineAfterNavigatingProjects(t *testing.T) {
	m, save, canonical := submittedReviewSave(t)
	m.selectedProject = &github.Project{Number: 2}
	m.view = &github.View{ProjectID: "other", Number: 4, Layout: github.BoardLayout}
	m.items = []github.Item{{ID: "other-item"}}
	updated, cmd := m.Update(save())
	m = updated.(Model)
	if cmd != nil || len(m.items) != 1 || m.items[0].ID != "other-item" {
		t.Fatalf("other project changed: %#v", m.items)
	}
	if (*canonical)[0].FieldValues[0].OptionID != "b" || m.mutationSession != nil {
		t.Fatal("original save did not complete")
	}
}

func TestSavePreservesOverlappingPagesAndRefreshesAfterwards(t *testing.T) {
	m, save, _ := submittedReviewSave(t)
	updated, _ := m.Update(keyPress("r"))
	m = updated.(Model)
	scope := m.currentBoardReadScope()
	m.items = []github.Item{{ID: "partial"}}
	updated, cmd := m.Update(save())
	m = updated.(Model)
	if cmd != nil || !m.itemsLoading || m.items[0].ID != "partial" || m.pendingMutationReload == nil {
		t.Fatal("save disturbed active read")
	}
	updated, cmd = m.Update(itemsPageMsg{scope: scope, generation: m.generation, page: github.ItemsPage{Items: []github.Item{{ID: "last-page"}}}})
	m = updated.(Model)
	if cmd == nil || !m.itemsLoading || m.pendingMutationReload != nil {
		t.Fatal("overlapping read was not followed by a fresh read")
	}
}

func TestSaveNeverProjectsOldViewFieldsIntoNewView(t *testing.T) {
	m, save, _ := submittedReviewSave(t)
	replacement := *m.view
	replacement.Number = 9
	m.view = &replacement
	m.items = []github.Item{{ID: "one", FieldValues: []github.FieldValue{{FieldID: "new-view-field", Value: "keep"}}}}
	updated, cmd := m.Update(save())
	m = updated.(Model)
	if cmd == nil || !m.itemsLoading {
		t.Fatal("new view was not refreshed using its own fields")
	}
	if len(m.items) != 0 {
		t.Fatal("old view items leaked into new view")
	}
}

func TestIssueReadbackKeepsGateUntilDesiredStateConfirmed(t *testing.T) {
	m := mutationModel(fakePickerSource{}, writableStatusView(), nil)
	m.detailVisible = true
	m.detailItemID = "one"
	m.detail = &github.ItemDetail{Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN"}}
	m.issueActionPending = &issueActionState{id: 7, itemID: "one", issueID: "issue", closed: true, blocked: true, scope: m.currentBoardReadScope(), readID: 1}
	m.mutationLoading = true
	originalScope := m.issueActionPending.scope
	for _, result := range []issueReadbackMsg{
		{actionID: 7, readID: 1, err: errors.New("unavailable")},
		{actionID: 7, readID: 1, detail: &github.ItemDetail{Content: &github.Content{ID: "issue", State: "OPEN"}}},
		{actionID: 7, readID: 1, detail: &github.ItemDetail{Content: &github.Content{ID: "different", State: "CLOSED"}}},
	} {
		updated, cmd := m.Update(result)
		m = updated.(Model)
		if cmd != nil || m.issueActionPending == nil || !m.mutationLoading {
			t.Fatal("uncertain readback unlocked writes")
		}
	}
	updated, _ := m.Update(keyPress("esc"))
	m = updated.(Model)
	if m.detailVisible || m.issueActionPending == nil {
		t.Fatal("escape discarded submitted action")
	}
	m.selectedProject = &github.Project{Number: 9}
	updated, retry := m.Update(keyPress("r"))
	m = updated.(Model)
	if retry == nil || m.issueActionPending.scope != originalScope {
		t.Fatal("retry lost originating project")
	}
	result := issueReadbackMsg{actionID: 7, readID: m.issueActionPending.readID, detail: &github.ItemDetail{Content: &github.Content{ID: "issue", State: "CLOSED"}}}
	updated, _ = m.Update(result)
	m = updated.(Model)
	if m.issueActionPending != nil || m.mutationLoading {
		t.Fatal("confirmed outcome kept gate locked")
	}
}

func TestTableActionUsesUnfilteredOriginReadbackAfterNavigation(t *testing.T) {
	canonical := []github.Item{{ID: "one"}}
	source := &fakeTableActionSource{fakePickerSource: fakePickerSource{canonicalItems: &canonical}, err: &github.MutationError{Err: errors.New("timeout"), Ambiguous: true}}
	m := tableActionModel(source)
	m.view.Filter = "is:issue"
	updated, _ := m.Update(keyPress("D"))
	m = updated.(Model)
	updated, write := m.Update(keyPress("enter"))
	m = updated.(Model)
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(Model)
	m.selectedProject = &github.Project{Number: 2}
	updated, readback := m.Update(write())
	m = updated.(Model)
	if readback == nil || m.tableAction == nil {
		t.Fatal("navigation lost submitted write")
	}
	updated, _ = m.Update(readback())
	m = updated.(Model)
	if m.tableAction == nil || m.tableAction.phase != "blocked" {
		t.Fatal("filtered view absence unlocked uncertain action")
	}
	updated, retry := m.Update(keyPress("r"))
	m = updated.(Model)
	if retry == nil {
		t.Fatal("retry not available from another screen")
	}
	canonical = nil
	updated, cmd := m.Update(retry())
	m = updated.(Model)
	if m.tableAction != nil || cmd != nil {
		t.Fatal("origin reconciliation disturbed other project")
	}
	if len(source.remove) != 1 {
		t.Fatal("write was replayed")
	}
}

func TestDetailRenderCacheInvalidation(t *testing.T) {
	m := NewModel(nil)
	m.width, m.height = 100, 40
	m.detail = &github.ItemDetail{Content: &github.Content{Kind: "Issue", State: "OPEN", Title: "first", BodyAvailable: true, Body: "**body**"}}
	first := m.renderedDetailLines()
	if len(first) == 0 {
		t.Fatal("empty fixture")
	}
	if &first[0] != &m.renderedDetailLines()[0] {
		t.Fatal("same document was rebuilt")
	}
	m.moveDetail(1)
	if &first[0] != &m.renderedDetailLines()[0] {
		t.Fatal("scroll rebuilt document")
	}
	for _, change := range []func(){
		func() { m.width = 80 },
		func() { m.detailShowAll = true },
		func() { m.detail.Content.State = "CLOSED" },
		func() {
			copied := *m.detail
			copied.CommentsLoaded = true
			copied.Comments = []github.IssueComment{{Body: "new comment"}}
			m.detail = &copied
		},
	} {
		change()
		next := m.renderedDetailLines()
		if &first[0] == &next[0] {
			t.Fatal("changed document retained stale render")
		}
		first = next
	}
	if !strings.Contains(ansi.Strip(strings.Join(first, "\n")), "new comment") {
		t.Fatal("fresh comments missing")
	}
}

func TestTerminalCellWidthForTextAndWrapping(t *testing.T) {
	for _, value := range []string{strings.Repeat("界", 12), strings.Repeat("👩‍💻", 6), strings.Repeat("e\u0301", 12), "ASCII words wrap here"} {
		for _, width := range []int{1, 3, 8, 12} {
			lines := append(wrapText(value, width), truncateText(value, width))
			for _, line := range lines {
				if !utf8.ValidString(line) || lipgloss.Width(line) > width {
					t.Fatalf("width %d text %q produced %q (%d cells)", width, value, line, lipgloss.Width(line))
				}
			}
		}
	}
	if got := wrapText("one two three", 7); !reflect.DeepEqual(got, []string{"one two", "three"}) {
		t.Fatalf("ASCII wrapping changed: %#v", got)
	}
}

func TestSwimlaneWindowAtBothEndsAndSmallSizes(t *testing.T) {
	view := writableCombinedView()
	view.VerticalGroupBy[0].Options = nil
	var items []github.Item
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("row-%d", i)
		view.VerticalGroupBy[0].Options = append(view.VerticalGroupBy[0].Options, github.FieldOption{ID: id, Name: strings.Repeat("long row name ", 12) + id})
		item := combinedItem(id, "p0", id)
		item.Content = &github.Content{Kind: "Issue", Title: fmt.Sprintf("TARGET-%d", i)}
		items = append(items, item)
	}
	for _, height := range []int{24, 30, 50, 70} {
		for _, index := range []int{0, 3, 5} {
			m := mutationModel(fakePickerSource{}, view, items)
			m.width, m.height = 100, height
			m.boardFocusID = items[index].ID
			m.clampBoardCursor()
			content := ansi.Strip(m.View().Content)
			lines := strings.Split(content, "\n")
			visible := strings.Join(lines[:min(len(lines), height)], "\n")
			if !strings.Contains(visible, fmt.Sprintf("TARGET-%d", index)) {
				t.Fatalf("height %d selected row %d hidden (%d total lines)", height, index, len(lines))
			}
			if len(lines) > height {
				t.Fatalf("height %d selected row %d produced %d lines: %q", height, index, len(lines), content)
			}
		}
	}
}

type reviewIssueSource struct {
	fakePickerSource
	err   error
	calls []bool
}

func (s *reviewIssueSource) IssueComment(_ context.Context, _ string, _ string) error { return nil }
func (s *reviewIssueSource) SetIssueClosed(_ context.Context, id string, closed bool) error {
	if id != "issue" {
		return errors.New("wrong issue identity")
	}
	s.calls = append(s.calls, closed)
	return s.err
}

func TestIssueSubmissionReconcilesWithoutReplayingWrite(t *testing.T) {
	source := &reviewIssueSource{err: &github.MutationError{Err: errors.New("timeout"), Ambiguous: true}}
	source.detail = github.ItemDetail{Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN"}}
	m := mutationModel(source.fakePickerSource, writableStatusView(), nil)
	m.source = source
	m.detailVisible = true
	m.detailItemID = "one"
	m.detail = &github.ItemDetail{Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN"}}
	updated, _ := m.Update(keyPress("x"))
	m = updated.(Model)
	if !m.issueActionConfirm {
		t.Fatal("close confirmation missing")
	}
	updated, write := m.Update(keyPress("enter"))
	m = updated.(Model)
	if write == nil || m.issueActionPending == nil || !m.mutationLoading {
		t.Fatal("write was not tracked")
	}
	updated, read := m.Update(write())
	m = updated.(Model)
	if read == nil {
		t.Fatal("uncertain write did not start readback")
	}
	updated, _ = m.Update(read())
	m = updated.(Model)
	if m.issueActionPending == nil || !m.mutationLoading {
		t.Fatal("unchanged state unlocked action")
	}
	source.detail.Content.State = "CLOSED"
	updated, read = m.Update(keyPress("r"))
	m = updated.(Model)
	updated, _ = m.Update(read())
	m = updated.(Model)
	if m.issueActionPending != nil || m.mutationLoading || m.detail.Content.State != "CLOSED" {
		t.Fatal("confirmed close did not reconcile")
	}
	if !reflect.DeepEqual(source.calls, []bool{true}) {
		t.Fatalf("writes = %#v", source.calls)
	}
}

func TestPickerNavigationKeysDoNotCancelStartup(t *testing.T) {
	for _, key := range []string{"p", "esc"} {
		m := NewModel(fakePickerSource{})
		ctx := m.ctx
		updated, _ := m.Update(keyPress(key))
		m = updated.(Model)
		if ctx.Err() != nil || m.generation != 0 {
			t.Fatalf("%s cancelled startup", key)
		}
	}
}
