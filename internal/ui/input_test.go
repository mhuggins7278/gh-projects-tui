package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestSearchEditingKeepsPickerAndBoardRouting(t *testing.T) {
	for _, mode := range []string{"picker", "board", "table"} {
		t.Run(mode, func(t *testing.T) {
			m := NewModel(nil)
			m.screen = screenOwnerPicker
			m.discovery = github.Discovery{Owners: []github.Owner{{Login: "Alpha"}, {Login: "Alpine"}}}
			if mode != "picker" {
				m.screen = screenBoard
				layout := github.BoardLayout
				if mode == "table" {
					layout = github.TableLayout
				}
				m.view = &github.View{Layout: layout}
				m.items = []github.Item{{ID: "one", Content: &github.Content{Title: "Alpha"}}, {ID: "two", Content: &github.Content{Title: "Alpine"}}}
			}
			m.filtering = true
			m.filter = "Al界"
			updated, _ := m.Update(keyPress("backspace"))
			m = updated.(Model)
			if m.filter != "Al" {
				t.Fatalf("backspace=%q", m.filter)
			}
			updated, _ = m.Update(keyPress("down"))
			m = updated.(Model)
			if mode == "picker" && m.cursor != 1 {
				t.Fatal("picker arrow did not navigate matches")
			}
			if mode != "picker" && (m.boardCard != 0 || m.tableRow != 0) {
				t.Fatal("board arrow navigated while editing search")
			}
			updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '?', Text: "?"}))
			m = updated.(Model)
			if m.filter != "Al?" || m.showHelp {
				t.Fatal("search text was interpreted as a shortcut")
			}
			updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
			m = updated.(Model)
			if m.filter != "" || !m.filtering || m.cursor != 0 {
				t.Fatal("clear did not reset search focus")
			}
			updated, _ = m.Update(tea.PasteMsg{Content: "Alpha\r\n界"})
			m = updated.(Model)
			if m.filter != "Alpha 界" {
				t.Fatalf("single line paste=%q", m.filter)
			}
			updated, _ = m.Update(keyPress("enter"))
			m = updated.(Model)
			if m.filtering || m.filter != "Alpha 界" || m.detailVisible {
				t.Fatal("accepting search selected an item or discarded text")
			}
			m.filtering = true
			updated, _ = m.Update(keyPress("esc"))
			m = updated.(Model)
			if m.filtering || m.filter != "" || (mode != "picker" && m.screen != screenBoard) {
				t.Fatal("cancel left search or navigated away")
			}
		})
	}
}

func TestCommentEditorPreservesMultilineTextAndModalEscape(t *testing.T) {
	m := NewModel(nil)
	m.screen = screenBoard
	m.detailVisible = true
	m.commentEditing = true
	m.detail = &github.ItemDetail{Content: &github.Content{Kind: "Issue"}}
	updated, _ := m.Update(tea.PasteMsg{Content: "First\nSecond界"})
	m = updated.(Model)
	updated, _ = m.Update(keyPress("backspace"))
	m = updated.(Model)
	if m.commentDraft != "First\nSecond" {
		t.Fatalf("comment text=%q", m.commentDraft)
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(Model)
	if m.commentEditing || m.commentDraft != "" || !m.detailVisible {
		t.Fatal("editor escape closed detail instead of cancelling the draft")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(Model)
	if m.detailVisible || m.detail != nil || m.screen != screenBoard {
		t.Fatal("detail escape navigated away from the board")
	}
}
