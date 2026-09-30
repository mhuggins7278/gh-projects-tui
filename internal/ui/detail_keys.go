package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func (m Model) updateDetailKey(key string) (tea.Model, tea.Cmd) {
	if m.mutationLoading {
		if key == "esc" && m.issueActionPending != nil && m.issueActionPending.blocked {
			m.detailVisible, m.detailLoading = false, false
			m.detailRequestID++
		}
		return m, nil
	}
	if m.issueActionConfirm {
		return m.updateIssueConfirmationKey(key)
	}
	if m.commentEditing {
		return m.updateCommentKey(key)
	}
	switch key {
	case "esc":
		m.detailRequestID++
		m.clearDetail()
		return m, nil
	case "o":
		return m, m.openBrowserCmd()
	case "c":
		if m.tableAction != nil || m.mutationSession != nil || m.issueActionPending != nil {
			m.status = "Finish or reconcile the pending project change before another issue write"
			return m, nil
		}
		if m.isTable() {
			return m, nil
		}
		if m.detail != nil && m.detail.Content != nil && m.detail.Content.Kind == "Issue" {
			m.commentEditing = true
			m.commentDraft = ""
			m.status = fmt.Sprintf("Comment on #%d: %s", m.detail.Content.Number, m.detail.Content.Title)
		} else {
			m.status = "Comments are available for issues only"
		}
		return m, nil
	case "x":
		if m.tableAction != nil || m.mutationSession != nil || m.issueActionPending != nil {
			m.status = "Finish or reconcile the pending project change before another issue write"
			return m, nil
		}
		if m.isTable() {
			return m, nil
		}
		if m.detail == nil || m.detail.Content == nil || m.detail.Content.Kind != "Issue" {
			m.status = "Close/reopen is available for issues only"
			return m, nil
		}
		if !strings.EqualFold(m.detail.Content.State, "OPEN") && !strings.EqualFold(m.detail.Content.State, "CLOSED") {
			m.status = "Issue state is unavailable; refresh details before changing it"
			return m, nil
		}
		if _, ok := m.source.(IssueActionSource); !ok {
			m.status = "Issue actions are unavailable for this client"
			return m, nil
		}
		m.issueActionClosing = strings.EqualFold(m.detail.Content.State, "OPEN")
		m.issueActionConfirm = true
		verb := "reopen"
		if m.issueActionClosing {
			verb = "close"
		}
		m.status = fmt.Sprintf("Press enter to %s issue #%d: %s · esc cancels", verb, m.detail.Content.Number, m.detail.Content.Title)
		return m, nil
	case "j", "down", "J":
		m.moveDetail(1)
		return m, nil
	case "k", "up", "K":
		m.moveDetail(-1)
		return m, nil
	case "g":
		m.detailOffset = 0
		return m, nil
	case "f", "F":
		m.detailShowAll = !m.detailShowAll
		m.detailOffset = 0
		return m, nil
	}
	return m, nil
}

func (m Model) updateIssueConfirmationKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.issueActionConfirm = false
		m.status = "Issue action cancelled"
		return m, nil
	case "enter":
		m.issueActionConfirm = false
		source, ok := m.source.(IssueActionSource)
		if !ok {
			m.status = "Issue actions are unavailable for this client"
			return m, nil
		}
		closed := m.issueActionClosing
		m.nextIssueAction++
		pending := &issueActionState{id: m.nextIssueAction, itemID: m.detailItemID, issueID: m.detail.Content.ID, scope: m.currentBoardReadScope(), closed: closed}
		m.issueActionPending = pending
		id, itemID, gen := m.detail.Content.ID, m.detailItemID, m.generation
		m.mutationLoading = true
		label := "Issue reopened."
		if closed {
			label = "Issue closed."
		}
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
			defer cancel()
			err := source.SetIssueClosed(ctx, id, closed)
			return issueActionMsg{actionID: pending.id, generation: gen, itemID: itemID, err: err, status: label, closed: &closed}
		}
	default:
		return m, nil
	}
}

func (m Model) updateCommentKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.commentEditing = false
		m.commentDraft = ""
		return m, nil
	case "enter":
		body := strings.TrimSpace(m.commentDraft)
		if body == "" {
			m.status = "Comment cannot be empty"
			return m, nil
		}
		if m.detail == nil || m.detail.Content == nil || m.detail.Content.Kind != "Issue" {
			m.status = "Comments are available for issues only"
			return m, nil
		}
		source, ok := m.source.(IssueActionSource)
		if !ok {
			m.status = "Issue actions are unavailable for this client"
			return m, nil
		}
		m.commentEditing = false
		m.commentDraft = ""
		m.mutationLoading = true
		id, itemID, gen := m.detail.Content.ID, m.detailItemID, m.generation
		return m, func() tea.Msg {
			err := source.IssueComment(context.Background(), id, body)
			status := "Comment added."
			if github.IsAmbiguousMutationError(err) {
				status = "Comment submission outcome is uncertain; check GitHub before retrying."
			}
			return issueActionMsg{generation: gen, itemID: itemID, err: err, status: status}
		}
	case "backspace":
		m.commentDraft = deleteLastRune(m.commentDraft)
		return m, nil
	default:
		return m, nil
	}
}
