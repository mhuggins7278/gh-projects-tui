package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type issueActionState struct {
	id              uint64
	itemID, issueID string
	scope           boardReadScope
	closed, blocked bool
	readID          uint64
}
type issueReadbackMsg struct {
	actionID, readID uint64
	state            github.IssueState
	err              error
}

type IssueStateSource interface {
	ReadIssueState(context.Context, string) (github.IssueState, error)
}

func (scope boardReadScope) sameProject(m Model) bool {
	return scope.host == m.host && m.selectedOwner != nil && m.selectedProject != nil &&
		scope.owner == *m.selectedOwner && scope.projectNumber == m.selectedProject.Number
}
func (m *Model) startIssueReadback() tea.Cmd {
	pending := m.issueActionPending
	if pending == nil || m.issueActionRefreshing {
		return nil
	}
	pending.readID++
	actionID, readID, issueID := pending.id, pending.readID, pending.issueID
	source := m.source
	m.issueActionRefreshing = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
		defer cancel()
		loader, ok := source.(IssueStateSource)
		if !ok {
			return issueReadbackMsg{actionID: actionID, readID: readID, err: fmt.Errorf("issue readback unavailable")}
		}
		state, err := loader.ReadIssueState(ctx, issueID)
		return issueReadbackMsg{actionID: actionID, readID: readID, state: state, err: err}
	}
}
func (m Model) updateIssueReadback(msg issueReadbackMsg) (tea.Model, tea.Cmd) {
	pending := m.issueActionPending
	if pending == nil || msg.actionID != pending.id || msg.readID != pending.readID {
		return m, nil
	}
	m.issueActionRefreshing = false
	wanted := "OPEN"
	if pending.closed {
		wanted = "CLOSED"
	}
	confirmed := msg.err == nil && msg.state.ID == pending.issueID && strings.EqualFold(msg.state.State, wanted)
	if !confirmed {
		pending.blocked = true
		m.mutationLoading = true
		m.status = "Issue outcome unknown; press r to check again, esc returns to the board. Further writes are blocked."
		return m, nil
	}
	delete(m.detailCache, pending.itemID)
	if pending.scope.sameProject(m) {
		for i := range m.items {
			if m.items[i].ID == pending.itemID && m.items[i].Content != nil {
				m.items[i].Content.State = wanted
			}
		}
		if m.detailItemID == pending.itemID && m.detail != nil && m.detail.Content != nil && m.detail.Content.ID == pending.issueID {
			m.detail.Content.State = wanted
		}
	}
	m.issueActionPending = nil
	m.mutationLoading = false
	m.status = "Issue action reconciled: " + strings.ToLower(wanted)
	return m, nil
}

func (m Model) updateIssueAction(msg issueActionMsg) (tea.Model, tea.Cmd) {
	if pending := m.issueActionPending; pending != nil {
		if msg.actionID != pending.id || msg.itemID != pending.itemID {
			return m, nil
		}
		if github.IsAmbiguousMutationError(msg.err) {
			pending.blocked = true
			m.mutationLoading = true
			m.status = "Issue outcome unknown; checking its state before another write."
			return m, m.startIssueReadback()
		}
		m.issueActionPending = nil
		m.issueActionRefreshing = false
		m.mutationLoading = false
		delete(m.detailCache, msg.itemID)
		if !pending.scope.sameProject(m) || !m.detailVisible || m.detailItemID != msg.itemID {
			return m, nil
		}
	} else if msg.actionID != 0 || msg.generation != m.generation || msg.itemID != m.detailItemID {
		return m, nil
	}
	m.mutationLoading = false
	if msg.err != nil {
		if github.IsAmbiguousMutationError(msg.err) {
			if msg.closed == nil {
				m.status = msg.status
				return m, nil
			}
			m.status = "Issue outcome unknown; refresh details before making another change."
			return m, nil
		}
		m.status = "Issue action failed: " + msg.err.Error()
		return m, nil
	}
	m.status = msg.status
	if msg.closed != nil {
		state := "OPEN"
		if *msg.closed {
			state = "CLOSED"
		}
		if m.detail != nil && m.detail.Content != nil {
			m.detail.Content.State = state
		}
		for i := range m.items {
			if m.items[i].ID == msg.itemID && m.items[i].Content != nil {
				m.items[i].Content.State = state
			}
		}
	}
	m.detailRequestID++
	return m, m.loadItemDetailCmd(m.detailItemID, m.detailRequestID, m.currentBoardReadScope())
}
