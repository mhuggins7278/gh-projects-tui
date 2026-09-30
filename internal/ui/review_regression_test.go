package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestReviewTextInputKeepsQ(t *testing.T) {
	for _, mode := range []string{"search", "comment"} {
		t.Run(mode, func(t *testing.T) {
			m := NewModel(nil)
			if mode == "search" {
				m.screen = screenOwnerPicker
				m.filtering = true
			} else {
				m.screen = screenBoard
				m.detailVisible = true
				m.commentEditing = true
				m.detail = &github.ItemDetail{Content: &github.Content{Kind: "Issue"}}
			}
			updated, cmd := m.Update(keyPress("q"))
			m = updated.(Model)
			if cmd != nil {
				if _, quit := cmd().(tea.QuitMsg); quit {
					t.Fatal("typing q produced tea.QuitMsg")
				}
			}
			if m.filter != "q" && m.commentDraft != "q" {
				t.Fatal("q was not entered")
			}
		})
	}
}

func TestReviewBackRejectsPendingView(t *testing.T) {
	source := fakePickerSource{view: github.View{ProjectID: "project", Number: 1, Layout: github.BoardLayout}}
	m := NewModel(source)
	m.screen = screenViewPicker
	m.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	m.selectedProject = &github.Project{Number: 1}
	m.views = []github.ViewSummary{{Number: 1, Layout: github.BoardLayout}}
	updated, open := m.Update(keyPress("enter"))
	m = updated.(Model)
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(Model)
	updated, _ = m.Update(open())
	m = updated.(Model)
	if m.screen != screenProjectPicker {
		t.Fatalf("late view completion reopened screen %d; wanted project picker", m.screen)
	}
}

func TestReviewRefreshPreservesSubmittedSave(t *testing.T) {
	view := writableStatusView()
	view.Layout = github.BoardLayout
	canonical := []github.Item{{ID: "one", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
	m := mutationModel(fakePickerSource{canonicalItems: &canonical}, view, canonical)
	updated, settle := m.Update(keyPress("L"))
	m = updated.(Model)
	updated, preflight := m.Update(settle())
	m = updated.(Model)
	updated, save := m.Update(preflight())
	m = updated.(Model)
	updated, refresh := m.Update(keyPress("r"))
	m = updated.(Model)
	if refresh == nil {
		t.Fatal("fixture did not start refresh")
	}
	updated, _ = m.Update(save())
	m = updated.(Model)
	if len(m.items) == 0 && !m.itemsLoading {
		t.Fatalf("save completion erased board and cancelled refresh; GitHub fixture still has %d item(s)", len(canonical))
	}
}

func TestReviewAmbiguousCloseKeepsWriteGate(t *testing.T) {
	m := NewModel(&commentActionPickerSource{})
	m.screen = screenBoard
	m.detailVisible = true
	m.detailItemID = "one"
	m.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	m.selectedProject = &github.Project{Number: 1}
	m.view = &github.View{ProjectID: "project", Number: 1, Layout: github.BoardLayout}
	m.detail = &github.ItemDetail{Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN"}}
	closed := true
	m.issueActionPending = &issueActionState{id: 1, itemID: "one", issueID: "issue", scope: m.currentBoardReadScope(), closed: true}
	updated, _ := m.Update(issueActionMsg{actionID: 1, generation: m.generation, itemID: "one", closed: &closed, err: &github.MutationError{Err: errors.New("timeout"), Ambiguous: true}})
	m = updated.(Model)
	updated, _ = m.Update(issueReadbackMsg{actionID: 1, readID: m.issueActionPending.readID, err: errors.New("readback unavailable")})
	m = updated.(Model)
	updated, _ = m.Update(keyPress("x"))
	m = updated.(Model)
	if m.issueActionConfirm || !m.mutationLoading {
		t.Fatalf("failed readback unlocked issue action: confirmation=%v loading=%v", m.issueActionConfirm, m.mutationLoading)
	}
}

func TestReviewPickerAcceptsUnicode(t *testing.T) {
	m := NewModel(nil)
	m.screen = screenProjectPicker
	m.filtering = true
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '界', Text: "界"}))
	m = updated.(Model)
	if m.filter != "界" {
		t.Fatalf("Unicode text dropped: %q", m.filter)
	}
}

func TestReviewSearchAcceptsSpaces(t *testing.T) {
	for _, screen := range []screen{screenProjectPicker, screenBoard} {
		m := NewModel(nil)
		m.screen = screen
		m.filtering = true
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
		m = updated.(Model)
		if m.filter != " " {
			t.Errorf("screen %d dropped a space: %q", screen, m.filter)
		}
	}
}

func TestReviewTextInputAcceptsPaste(t *testing.T) {
	for _, mode := range []string{"search", "comment"} {
		t.Run(mode, func(t *testing.T) {
			m := NewModel(nil)
			m.screen = screenBoard
			if mode == "search" {
				m.filtering = true
			} else {
				m.detailVisible = true
				m.commentEditing = true
				m.detail = &github.ItemDetail{Content: &github.Content{Kind: "Issue"}}
			}
			updated, _ := m.Update(tea.PasteMsg{Content: "pasted text"})
			m = updated.(Model)
			if m.filter != "pasted text" && m.commentDraft != "pasted text" {
				t.Fatal("pasted text was discarded")
			}
		})
	}
}

func TestReviewUnknownTableActionSurvivesViewChange(t *testing.T) {
	source := &fakeTableActionSource{}
	m := tableActionModel(source)
	items := cloneItems(m.items)
	view := *m.view
	m.tableAction = &tableActionState{scope: m.currentBoardReadScope(), itemID: "one", remove: true, phase: "blocked"}
	updated, _ := m.Update(keyPress("esc"))
	m = updated.(Model)
	m.screen = screenViewPicker
	updated, _ = m.Update(viewDetailMsg{scope: m.currentPickerReadScope(), generation: m.generation, view: &view})
	m = updated.(Model)
	updated, _ = m.Update(itemsPageMsg{generation: m.generation, scope: m.currentBoardReadScope(), reset: true, page: github.ItemsPage{Items: items}})
	m = updated.(Model)
	updated, _ = m.Update(keyPress("D"))
	m = updated.(Model)
	if m.tableAction == nil || m.tableAction.phase != "blocked" {
		t.Fatal("same uncertain removal can be submitted again after returning to the view")
	}
}

func TestReviewTruncationFitsTerminalCells(t *testing.T) {
	value := truncateText(strings.Repeat("界", 12), 12)
	if width := lipgloss.Width(value); width > 12 {
		t.Fatalf("12-column truncation produced %d terminal cells", width)
	}
}

func TestReviewSwimlaneSelectionFitsViewport(t *testing.T) {
	view := writableCombinedView()
	view.VerticalGroupBy[0].Options = nil
	var items []github.Item
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("s%d", i)
		view.VerticalGroupBy[0].Options = append(view.VerticalGroupBy[0].Options, github.FieldOption{ID: id, Name: id})
		item := combinedItem(id, "p0", id)
		item.Content = &github.Content{Title: fmt.Sprintf("ROW-%d-TITLE", i), Kind: "Issue"}
		items = append(items, item)
	}
	m := mutationModel(fakePickerSource{}, view, items)
	m.width = 100
	m.height = 30
	m.boardFocusID = "s5"
	m.clampBoardCursor()
	content := m.View().Content
	lines := strings.Split(content, "\n")
	visible := strings.Join(lines[:min(len(lines), m.height)], "\n")
	if !strings.Contains(ansi.Strip(visible), "ROW-5-TITLE") {
		t.Fatalf("selected last swimlane is outside first %d terminal lines (rendered %d lines)", m.height, len(lines))
	}
}

func BenchmarkReviewDetailScroll(b *testing.B) {
	for _, count := range []int{20, 100} {
		b.Run(fmt.Sprintf("comments-%d", count), func(b *testing.B) {
			m := benchmarkDetailModel(count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				updated, _ := m.Update(keyPress("j"))
				m = updated.(Model)
				_ = m.View().Content
			}
		})
	}
}

func benchmarkDetailModel(count int) Model {
	m := NewModel(nil)
	m.screen = screenBoard
	m.width = 100
	m.height = 40
	m.detailVisible = true
	m.view = &github.View{Layout: github.BoardLayout}
	m.detail = &github.ItemDetail{Content: &github.Content{Kind: "Issue", Title: "Representative issue", BodyAvailable: true, Body: strings.Repeat("A paragraph with **markdown** and `code`. ", 50)}, CommentsLoaded: true}
	for i := 0; i < count; i++ {
		m.detail.Comments = append(m.detail.Comments, github.IssueComment{Author: "viewer", CreatedAt: "2026-09-29T12:00:00Z", Body: strings.Repeat("A reply with **markdown** and `code`. ", 25)})
	}
	return m
}

func BenchmarkCachedDetailScroll(b *testing.B) {
	for _, count := range []int{20, 100} {
		b.Run(fmt.Sprintf("comments-%d", count), func(b *testing.B) {
			m := benchmarkDetailModel(count)
			_ = m.renderedDetailLines()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				updated, _ := m.Update(keyPress("j"))
				m = updated.(Model)
				_ = m.View().Content
			}
		})
	}
}

func BenchmarkColdDetailRender(b *testing.B) {
	for _, n := range []int{20, 100} {
		b.Run(fmt.Sprintf("comments-%d", n), func(b *testing.B) {
			m := benchmarkDetailModel(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = detailLines(*m.detail, 80, false)
			}
		})
	}
}
