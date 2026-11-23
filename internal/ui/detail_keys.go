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
		if key == "esc" && (m.mutationSession != nil || m.issueActionPending != nil) {
			m.detailRequestID++
			m.clearDetail()
			return m, m.finishIssueViewReload()
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
		return m, m.finishIssueViewReload()
	case "o":
		return m, m.openBrowserCmd()
	case "e":
		m.beginDetailFieldEdit()
		return m, nil
	case "l":
		return m, m.beginIssueMetadataEdit(github.IssueLabels)
	case "a":
		return m, m.beginIssueMetadataEdit(github.IssueAssignees)
	case "c":
		if m.tableAction != nil || m.mutationSession != nil || m.issueActionPending != nil {
			m.status = "Finish or reconcile the pending project change before another issue write"
			return m, nil
		}
		if m.isTimeline() {
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
		m.beginIssueStateChange()
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

func (m *Model) beginIssueStateChange() {
	if m.tableAction != nil || m.mutationSession != nil || m.issueActionPending != nil || m.mutationLoading || m.itemsLoading || m.detailLoading {
		m.status = "Finish pending reads and writes before another issue change"
		return
	}
	if m.isTimeline() {
		return
	}
	var itemID string
	var content *github.Content
	if m.detailVisible {
		itemID = m.detailItemID
		if m.detail != nil {
			content = m.detail.Content
		}
	} else if item, ok := m.selectedItem(); ok {
		itemID, content = item.ID, item.Content
	}
	if content == nil || content.Kind != "Issue" || content.ID == "" || itemID == "" {
		m.status = "Close/reopen is available for issues only"
		return
	}
	if !strings.EqualFold(content.State, "OPEN") && !strings.EqualFold(content.State, "CLOSED") {
		m.status = "Issue state is unavailable; refresh before changing it"
		return
	}
	if _, ok := m.source.(IssueActionSource); !ok {
		m.status = "Issue actions are unavailable for this client"
		return
	}
	m.issueActionClosing = strings.EqualFold(content.State, "OPEN")
	permission := content.ViewerCanReopen
	if m.issueActionClosing {
		permission = content.ViewerCanClose
	}
	if permission != nil && !*permission {
		m.status = "You do not have permission to change this issue's state"
		return
	}
	m.issueActionTarget = &issueActionState{itemID: itemID, issueID: content.ID, scope: m.currentBoardReadScope(), closed: m.issueActionClosing, previousState: content.State}
	m.issueActionConfirm = true
	verb := "reopen"
	if m.issueActionClosing {
		verb = "close"
	}
	m.status = fmt.Sprintf("Press enter to %s issue #%d: %s · esc cancels", verb, content.Number, content.Title)
}

func (m Model) updateIssueConfirmationKey(key string) (tea.Model, tea.Cmd) {
	if m.isTimeline() {
		return m, nil
	}
	switch key {
	case "esc":
		m.issueActionConfirm = false
		m.issueActionTarget = nil
		m.status = ""
		return m, nil
	case "enter":
		m.issueActionConfirm = false
		source, ok := m.source.(IssueActionSource)
		if !ok {
			m.status = "Issue actions are unavailable for this client"
			return m, nil
		}
		pending := m.issueActionTarget
		m.issueActionTarget = nil
		// Older callers may already have opened the detail confirmation.
		if pending == nil && m.detail != nil && m.detail.Content != nil {
			pending = &issueActionState{itemID: m.detailItemID, issueID: m.detail.Content.ID, scope: m.currentBoardReadScope(), closed: m.issueActionClosing, previousState: m.detail.Content.State}
		}
		if pending == nil || !pending.scope.matches(m) {
			m.status = "Issue selection changed; try again"
			return m, nil
		}
		closed := pending.closed
		m.nextIssueAction++
		pending.id = m.nextIssueAction
		m.issueActionPending = pending
		id, itemID, gen := pending.issueID, pending.itemID, m.generation
		m.mutationLoading = true
		label := "Issue reopened."
		state := "OPEN"
		if closed {
			label = "Issue closed."
			state = "CLOSED"
		}
		m.applyIssueState(itemID, id, state)
		m.status = ""
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
	if m.isTimeline() {
		return m, nil
	}
	switch key {
	case "esc":
		m.commentEditing = false
		m.commentDraft = ""
		m.status = ""
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
		m.status = ""
		m.mutationRefreshID++
		m.mutationLoading = true
		id, itemID, gen := m.detail.Content.ID, m.detailItemID, m.generation
		commentID := fmt.Sprintf("pending-comment-%d", m.mutationRefreshID)
		if m.detail.CommentsLoaded {
			detail := *m.detail
			detail.Comments = append([]github.IssueComment(nil), m.detail.Comments...)
			m.detail = &detail
			m.detail.Comments = append(m.detail.Comments, github.IssueComment{ID: commentID, Author: "You (sending)", Body: body})
		}
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
			defer cancel()
			err := source.IssueComment(ctx, id, body)
			status := "Comment added."
			if github.IsAmbiguousMutationError(err) {
				status = "Comment submission outcome is uncertain; check GitHub before retrying."
			}
			return issueActionMsg{generation: gen, itemID: itemID, err: err, status: status, commentID: commentID}
		}
	case "backspace":
		m.commentDraft = deleteLastRune(m.commentDraft)
		return m, nil
	default:
		return m, nil
	}
}
