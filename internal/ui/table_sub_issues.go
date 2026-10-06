package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type subIssueSource interface {
	PageSubIssues(context.Context, string, string, string, []github.Field) (github.ItemsPage, error)
}

type subIssueState struct {
	expanded, loading, complete bool
	items                       []github.Item
	after                       string
	err                         error
	requestID                   uint64
}

type subIssuesMsg struct {
	scope           boardReadScope
	parentID, after string
	requestID       uint64
	page            github.ItemsPage
	err             error
}

type tableTreeRow struct {
	item     github.Item
	depth    int
	parentID string
	group    int
}

// Build a display hierarchy without adding child rows to saved-query membership
// or the canonical project mutation state. Only expanded, loaded paths participate.
func (m Model) tableTreeRows() []tableTreeRow {
	groups := tableGroups(m.view, sortedItemsForView(m.view, m.items))
	nested := map[string]bool{}
	var collect func(github.Item, map[string]bool)
	collect = func(item github.Item, path map[string]bool) {
		if path[item.ID] {
			return
		}
		path[item.ID] = true
		defer delete(path, item.ID)
		if state := m.tableSubIssues[item.ID]; state != nil && state.expanded {
			for _, child := range state.items {
				if !path[child.ID] {
					nested[child.ID] = true
					collect(child, path)
				}
			}
		}
	}
	for _, group := range groups {
		for _, item := range group.Items {
			collect(item, map[string]bool{})
		}
	}
	seen := map[string]bool{}
	repeatedGroups := m.view != nil && len(m.view.GroupByFields) == 1 && isMultiSelectField(m.view.GroupByFields[0])
	visitKey := func(group int, id string) string {
		if repeatedGroups {
			return fmt.Sprintf("%d/%s", group, id)
		}
		return id
	}
	var appendRow func(github.Item, int, string, int) []tableTreeRow
	appendRow = func(item github.Item, depth int, parent string, group int) []tableTreeRow {
		key := visitKey(group, item.ID)
		if seen[key] {
			return nil
		}
		seen[key] = true
		row := tableTreeRow{item: item, depth: depth, parentID: parent, group: group}
		children := []tableTreeRow{}
		if state := m.tableSubIssues[item.ID]; state != nil && state.expanded {
			for _, child := range state.items {
				children = append(children, appendRow(child, depth+1, item.ID, group)...)
			}
		}
		if strings.TrimSpace(m.filter) != "" && !itemMatchesBoardSearch(item, m.filter) && len(children) == 0 {
			return nil
		}
		return append([]tableTreeRow{row}, children...)
	}
	rows := []tableTreeRow{}
	for groupIndex, group := range groups {
		for _, item := range group.Items {
			if !nested[item.ID] {
				rows = append(rows, appendRow(item, 0, "", groupIndex)...)
			}
		}
	}
	// A malformed cyclic hierarchy must not hide every root. The visited set
	// still ensures each row appears once when recovering an unattached root.
	for groupIndex, group := range groups {
		for _, item := range group.Items {
			if !seen[visitKey(groupIndex, item.ID)] {
				rows = append(rows, appendRow(item, 0, "", groupIndex)...)
			}
		}
	}
	return rows
}

func (m Model) tableTreeGroups(rows []tableTreeRow) []boardLane {
	groups := tableGroups(m.view, sortedItemsForView(m.view, m.items))
	for index := range groups {
		groups[index].Items = nil
	}
	for _, row := range rows {
		groups[row.group].Items = append(groups[row.group].Items, row.item)
	}
	return groups
}

func (m *Model) expandTableRow() tea.Cmd {
	item, ok := m.selectedItem()
	if !ok || item.Content == nil || item.Content.Kind != "Issue" || item.Content.SubIssueTotal == 0 {
		return nil
	}
	if m.itemsLoading || m.loadingDetail {
		return nil
	}
	if m.tableSubIssues == nil {
		m.tableSubIssues = map[string]*subIssueState{}
	}
	state := m.tableSubIssues[item.ID]
	if state == nil {
		state = &subIssueState{}
		m.tableSubIssues[item.ID] = state
	}
	state.expanded = true
	m.tableFocusID = item.ID
	m.refreshFocusID = ""
	return m.loadSubIssues(item)
}

func (m *Model) collapseTableRow() {
	item, ok := m.selectedItem()
	if !ok {
		return
	}
	if state := m.tableSubIssues[item.ID]; state != nil && state.expanded {
		state.expanded = false
		m.tableFocusID = item.ID
	} else {
		for _, row := range m.tableTreeRows() {
			if row.item.ID == item.ID && row.parentID != "" {
				m.tableFocusID = row.parentID
				break
			}
		}
	}
	m.refreshFocusID = ""
	m.clampTableRow(true)
}

func (m *Model) loadSubIssues(item github.Item) tea.Cmd {
	state := m.tableSubIssues[item.ID]
	if state == nil || state.loading || state.complete || !state.expanded || item.Content == nil {
		return nil
	}
	loader, ok := m.source.(subIssueSource)
	if !ok {
		state.err = fmt.Errorf("sub-issue loading is unavailable for this client")
		m.status = state.err.Error()
		return nil
	}
	state.loading, state.err = true, nil
	state.requestID++
	requestID, after := state.requestID, state.after
	scope, ctx, issueID := m.currentBoardReadScope(), m.ctx, item.Content.ID
	fields := boardReadFields(*m.view)
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}
		page, err := loader.PageSubIssues(ctx, scope.projectID, issueID, after, fields)
		return subIssuesMsg{scope: scope, parentID: item.ID, requestID: requestID, after: after, page: page, err: err}
	}
}

func (m Model) updateSubIssues(msg subIssuesMsg) (tea.Model, tea.Cmd) {
	state := m.tableSubIssues[msg.parentID]
	if !m.isTable() || !msg.scope.matches(m) || m.loadingDetail || state == nil || state.requestID != msg.requestID || !state.loading {
		return m, nil
	}
	state.loading = false
	if msg.err == nil && msg.page.HasNext && (msg.page.EndCursor == "" || msg.page.EndCursor == msg.after) {
		msg.err = fmt.Errorf("sub-issue pagination did not advance")
	}
	if msg.err != nil {
		state.err = msg.err
		m.status = "Sub-issues failed: " + msg.err.Error() + "; press l/right on the parent to retry"
		m.clampTableRow(!m.subIssuesPending())
		return m, nil
	}
	seen := map[string]bool{}
	for _, item := range state.items {
		seen[item.ID] = true
	}
	for _, item := range msg.page.Items {
		if item.ID != "" && item.ID != msg.parentID && !seen[item.ID] {
			state.items = append(state.items, item)
			seen[item.ID] = true
		}
	}
	state.after, state.complete = msg.page.EndCursor, !msg.page.HasNext
	if strings.HasPrefix(m.status, "Sub-issues failed:") {
		m.status = ""
	}
	cmd := m.loadExpandedSubIssues()
	m.clampTableRow(!m.subIssuesPending())
	return m, cmd
}

func (m *Model) loadExpandedSubIssues() tea.Cmd {
	if !m.isTable() || m.itemsLoading || m.loadingDetail {
		return nil
	}
	commands := []tea.Cmd{}
	for _, row := range m.tableTreeRows() {
		if state := m.tableSubIssues[row.item.ID]; state != nil && state.expanded && state.err == nil {
			if cmd := m.loadSubIssues(row.item); cmd != nil {
				commands = append(commands, cmd)
			}
		}
	}
	return tea.Batch(commands...)
}

func (m Model) subIssuesPending() bool {
	for _, row := range m.tableTreeRows() {
		if state := m.tableSubIssues[row.item.ID]; state != nil && state.expanded && !state.complete && state.err == nil {
			return true
		}
	}
	return false
}

func (m *Model) resetSubIssueReads() {
	for id, state := range m.tableSubIssues {
		m.tableSubIssues[id] = &subIssueState{expanded: state.expanded, requestID: state.requestID + 1}
	}
}

func (m *Model) cancelSubIssueReads() {
	for _, state := range m.tableSubIssues {
		state.loading = false
		state.requestID++
	}
}

func (m Model) tableTreePrefix(row tableTreeRow) string {
	marker := "  "
	if row.item.Content != nil && row.item.Content.SubIssueTotal > 0 {
		marker = "▸ "
		if state := m.tableSubIssues[row.item.ID]; state != nil {
			switch {
			case state.loading:
				marker = "… "
			case state.err != nil:
				marker = "! "
			case state.expanded:
				marker = "▾ "
			}
		}
	}
	return strings.Repeat("  ", row.depth) + marker
}
