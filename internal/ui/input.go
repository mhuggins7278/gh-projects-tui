package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// Text payloads belong to the focused editor before printable shortcuts.
	if (m.filtering || m.commentEditing) && msg.Text != "" && key != "ctrl+c" {
		m.appendInputText(msg.Text)
		return m, nil
	}
	// Global keys.
	switch key {
	case "q", "ctrl+c":
		if key == "q" && (m.filtering || m.commentEditing) {
			return m, nil
		}
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	case "?":
		if !m.filtering && !m.commentEditing {
			m.showHelp = !m.showHelp
			return m, nil
		}
	case "A":
		if !m.filtering && !m.commentEditing && m.debug {
			m.showAPI = !m.showAPI
			return m, nil
		}
	}
	if key == "r" && m.tableAction != nil && m.tableAction.phase == "blocked" {
		return m, m.startTableActionReadback()
	}
	if key == "r" && m.issueActionPending != nil && m.issueActionPending.blocked {
		return m, m.startIssueReadback()
	}
	if key == "r" && m.mutationSession != nil && m.mutationSession.blocked {
		return m, m.retryMutationReadback()
	}

	if m.filtering {
		return m.updateSearchKey(key)
	}
	if m.screen == screenBoard {
		if m.isTable() && m.tableAction != nil && m.tableAction.phase == "confirm" {
			return m.updateTableConfirmationKey(key)
		}
		if m.isTable() && m.tableMove != nil {
			return m.updateTableMoveKey(key)
		}
		if m.detailVisible {
			return m.updateDetailKey(key)
		}
		if m.isTimeline() {
			if cmd, handled := m.handleTimelineKey(key); handled {
				return m, cmd
			}
		} else if m.isTable() {
			if cmd, handled := m.handleTableNavigationKey(key); handled {
				return m, cmd
			}
		} else {
			if cmd, handled := m.handleBoardNavigationKey(key); handled {
				return m, cmd
			}
		}
	}
	return m.updateNavigationKey(key)
}

func (m Model) updateNavigationKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "/":
		m.filtering = true
		if m.screen == screenBoard {
			m.filter = ""
			m.resetSearchCursor()
		}
		return m, nil
	case "esc":
		return m, m.goBack()
	case "backspace":
		return m, m.goBack()
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		return m, nil
	case "enter":
		return m, m.selectCurrent()
	case "p":
		if m.screen == screenProjectPicker || (m.selectedOwner == nil && m.screen != screenBoard && m.screen != screenViewPicker) {
			return m, nil
		}
		m.abandonReads()
		m.screen = screenProjectPicker
		m.cursor = 0
		m.filter = ""
		m.view = nil
		m.clearDetail()
		return m, nil
	case "v":
		if m.selectedProject != nil && m.selectedOwner != nil {
			cmd := (&m).startViewsLoad()
			return m, cmd
		}
		return m, nil
	case "r":
		return m, m.refresh()
	case "o":
		return m, m.openBrowserCmd()
	}
	return m, nil
}

var singleLineInput = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// appendInputText consumes text and paste payloads before printable shortcuts.
func (m *Model) appendInputText(text string) {
	if m.commentEditing && !m.mutationLoading {
		m.commentDraft += text
		return
	}
	if !m.filtering {
		return
	}
	m.filter += singleLineInput.Replace(text)
	m.resetSearchCursor()
}

func deleteLastRune(text string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}
	return string(runes[:len(runes)-1])
}

// resetSearchCursor keeps picker and board focus valid after an edit.
func (m *Model) resetSearchCursor() {
	m.refreshFocusID = ""
	m.cursor = 0
	if m.screen == screenBoard {
		if m.isRowLayout() {
			m.clampTableRow(true)
		} else {
			m.clampBoardCursor()
		}
	}
}

func (m *Model) updateSearchKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		m.filtering = false
	case "esc":
		m.filtering = false
		m.filter = ""
	case "backspace":
		if m.filter == "" {
			return *m, nil
		}
		m.filter = deleteLastRune(m.filter)
	case "ctrl+u":
		m.filter = ""
	default:
		if m.screen != screenBoard {
			switch key {
			case "up", "k":
				m.moveCursor(-1)
			case "down", "j":
				m.moveCursor(1)
			}
		}
		return *m, nil
	}
	m.resetSearchCursor()
	return *m, nil
}
