package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type tableItemActionSource interface {
	ArchiveProjectItem(context.Context, string, string) error
	RemoveProjectItem(context.Context, string, string) error
}

type tableActionState struct {
	scope  boardReadScope
	readID uint64
	itemID string
	label  string
	remove bool
	phase  string // confirm, saving, reconciling, blocked
}

type tableActionMsg struct {
	scope  boardReadScope
	itemID string
	remove bool
	err    error
}

type tableMoveState struct {
	itemID string
	lanes  []boardLane
	index  int
}

func tableMutationLanes(view *github.View, items []github.Item) []boardLane {
	lanes := lanesForView(view, items)
	if view != nil && len(view.GroupByFields) == 1 && strings.EqualFold(view.GroupByFields[0].Name, "Status") {
		if len(lanes) > 0 && lanes[0].Key != "no-value" {
			lanes = append([]boardLane{{Key: "no-value", Name: "No Status"}}, lanes...)
		}
	}
	return lanes
}

func (m *Model) selectTableBoardCursor() bool {
	item, ok := m.selectedItem()
	if !ok {
		m.status = "Select a table row first"
		return false
	}
	for laneIndex, lane := range m.boardLanes() {
		for cardIndex, card := range lane.Items {
			if card.ID == item.ID {
				m.boardLane, m.boardCard, m.boardFocusID = laneIndex, cardIndex, item.ID
				return true
			}
		}
	}
	m.status = "The selected row is no longer in this view; refresh first"
	return false
}

func (m *Model) beginTableMove() {
	if reason := m.boardMutationUnavailable(); reason != "" {
		m.status = reason
		return
	}
	if m.view == nil || len(m.view.GroupByFields) != 1 {
		m.status = "Moving between groups requires one saved table grouping field"
		return
	}
	if !m.selectTableBoardCursor() {
		return
	}
	item, _ := m.selectedItem()
	lanes := tableMutationLanes(m.view, m.boardItems())
	current := 0
	for index, lane := range lanes {
		for _, candidate := range lane.Items {
			if candidate.ID == item.ID {
				current = index
			}
		}
	}
	m.tableMove = &tableMoveState{itemID: item.ID, lanes: lanes, index: current}
	m.status = "Choose a destination group with j/k; enter moves, esc cancels"
}

func (m *Model) finishTableMove() tea.Cmd {
	move := m.tableMove
	m.tableMove = nil
	if move == nil || move.index < 0 || move.index >= len(move.lanes) {
		return nil
	}
	if reason := m.boardMutationUnavailable(); reason != "" {
		m.status = reason
		return nil
	}
	item, ok := m.selectedItem()
	if !ok || item.ID != move.itemID || !m.selectTableBoardCursor() {
		m.status = "Row selection changed; choose a destination again"
		return nil
	}
	current := 0
	for index, lane := range move.lanes {
		for _, candidate := range lane.Items {
			if candidate.ID == item.ID {
				current = index
			}
		}
	}
	if current == move.index {
		m.status = "Item is already in that group"
		return nil
	}
	// The intent resolves the destination against a fresh project read before
	// applying a field/position update; no stale table row indexes are written.
	return m.enqueueBoardMutation(boardMutationIntent{
		kind: boardMutationMoveLane, itemID: item.ID, direction: move.index - current,
		destination: move.lanes[move.index].Key, description: "Move table row to " + move.lanes[move.index].Name,
	})
}

func (m *Model) beginTableAction(remove bool) {
	if reason := m.tableActionUnavailable(); reason != "" {
		m.status = reason
		return
	}
	if _, ok := m.source.(tableItemActionSource); !ok {
		m.status = "Project item actions are unavailable for this client"
		return
	}
	item, ok := m.selectedItem()
	if !ok || item.ID == "" {
		m.status = "Select a table row first"
		return
	}
	label := item.ID
	if item.Content != nil {
		label = contentCardIdentity(item.Content) + ": " + item.Content.Title
	}
	m.tableAction = &tableActionState{scope: m.currentBoardReadScope(), itemID: item.ID, label: label, remove: remove, phase: "confirm"}
	verb := "archive"
	if remove {
		verb = "remove from this project"
		if item.Content != nil && item.Content.Kind == "DraftIssue" {
			m.status = "Remove project-only draft (deletes the draft itself)? Enter confirms; esc cancels: " + label
			return
		}
	}
	m.status = fmt.Sprintf("Press enter to %s %s · esc cancels", verb, label)
}

func (m Model) tableActionUnavailable() string {
	if m.issueActionPending != nil {
		return "Reconcile the pending issue action first"
	}
	if !m.isTable() || m.view.ProjectID == "" || m.selectedOwner == nil || m.selectedProject == nil {
		return "A loaded table and project identity are required"
	}
	if !m.view.ViewerCanUpdate {
		return "This project is read-only"
	}
	if m.tableAction != nil {
		return "Finish or reconcile the pending table action first"
	}
	if m.itemsLoading || m.itemsErr != nil || m.mutationLoading || m.mutationSession != nil {
		return "Wait for project items and pending saves to finish before acting"
	}
	if strings.TrimSpace(m.filter) != "" || m.filtering {
		return "Clear local search before changing project items"
	}
	return ""
}

func (m *Model) confirmTableAction() tea.Cmd {
	state := m.tableAction
	if state == nil || state.phase != "confirm" {
		return nil
	}
	if m.itemsLoading || m.mutationSession != nil {
		m.tableAction = nil
		m.status = "Project state changed; select the row again"
		return nil
	}
	state.phase = "saving"
	projectID := m.view.ProjectID
	itemID := state.itemID
	remove := state.remove
	scope := state.scope
	source := m.source.(tableItemActionSource)
	m.status = "Saving project item action..."
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
		defer cancel()
		var err error
		if remove {
			err = source.RemoveProjectItem(ctx, projectID, itemID)
		} else {
			err = source.ArchiveProjectItem(ctx, projectID, itemID)
		}
		return tableActionMsg{scope: scope, itemID: itemID, remove: remove, err: err}
	}
}

func (m Model) updateTableAction(msg tableActionMsg) (tea.Model, tea.Cmd) {
	state := m.tableAction
	if state == nil || state.phase != "saving" || msg.scope != state.scope || state.itemID != msg.itemID || state.remove != msg.remove {
		return m, nil
	}
	verb := "Archived"
	if state.remove {
		verb = "Removed from project"
	}
	if msg.err != nil && !github.IsAmbiguousMutationError(msg.err) {
		m.status = "Project item action failed: " + msg.err.Error()
		m.tableAction = nil
		return m, nil
	}
	if msg.err != nil {
		state.phase = "reconciling"
		m.status = "Outcome uncertain; checking active project items before any retry..."
		return m, m.startTableActionReadback()
	} else {
		m.tableAction = nil
		m.status = verb + ": " + state.label
	}
	// Select the next surviving row (or the previous one), even if the saved
	// view's grouping or sort places it elsewhere after the next read.
	if !state.scope.sameProject(m) || m.screen != screenBoard {
		return m, nil
	}
	items := m.tableItems()
	index := m.selectedTableRow(items)
	if index >= 0 && items[index].ID == msg.itemID {
		m.tableFocusID = ""
		if index+1 < len(items) {
			m.tableFocusID = items[index+1].ID
		} else if index > 0 {
			m.tableFocusID = items[index-1].ID
		}
	}
	delete(m.detailCache, msg.itemID)
	if invalidator, ok := m.source.(interface{ InvalidateReads() }); ok {
		invalidator.InvalidateReads()
	}
	if m.itemsLoading {
		scope := m.currentBoardReadScope()
		m.pendingMutationReload = &scope
		return m, nil
	}
	return m, m.startItemsLoadWithSpinner()
}

type tableActionReadbackMsg struct {
	scope  boardReadScope
	readID uint64
	items  []github.Item
	err    error
}

func (m *Model) startTableActionReadback() tea.Cmd {
	state := m.tableAction
	if state == nil || (state.phase != "blocked" && state.phase != "reconciling") {
		return nil
	}
	state.phase = "reconciling"
	state.readID++
	scope, readID, source := state.scope, state.readID, m.source
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
		defer cancel()
		loader, ok := source.(ItemsSource)
		if !ok {
			return tableActionReadbackMsg{scope: scope, readID: readID, err: fmt.Errorf("project item readback unavailable")}
		}
		items, err := readCanonicalProjectItems(ctx, loader, scope.owner, scope.projectNumber)
		return tableActionReadbackMsg{scope: scope, readID: readID, items: items, err: err}
	}
}
func (m Model) updateTableActionReadback(msg tableActionReadbackMsg) (tea.Model, tea.Cmd) {
	state := m.tableAction
	if state == nil || state.phase != "reconciling" || state.scope != msg.scope || state.readID != msg.readID {
		return m, nil
	}
	present := false
	for _, item := range msg.items {
		if item.ID == state.itemID {
			present = true
			break
		}
	}
	if msg.err != nil || present {
		state.phase = "blocked"
		m.status = "Project item action outcome unknown; press r to check again. Further writes remain blocked."
		return m, nil
	}
	m.tableAction = nil
	delete(m.detailCache, state.itemID)
	m.status = "Item no longer appears among the project's active items; action reconciled."
	if state.scope.sameProject(m) && m.screen == screenBoard {
		if m.itemsLoading {
			scope := m.currentBoardReadScope()
			m.pendingMutationReload = &scope
			return m, nil
		}
		return m, m.startItemsLoadWithSpinner()
	}
	return m, nil
}
