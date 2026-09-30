package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type viewRefreshMsg struct {
	scope boardReadScope
	view  *github.View
	err   error
}

func (m *Model) refreshSavedView() tea.Cmd {
	m.loadingDetail = true
	// Use the existing read gate during metadata loading too: submitted saves
	// must wait for the fresh view before triggering any item reload.
	m.itemsLoading = true
	m.status = "Refreshing saved view settings..."
	scope, ctx, source := m.currentBoardReadScope(), m.ctx, m.source
	return func() tea.Msg {
		loader, ok := source.(ViewSource)
		if !ok {
			return viewRefreshMsg{scope: scope, err: fmt.Errorf("view refresh is not supported by this client")}
		}
		view, err := loader.OpenView(ctx, scope.owner, scope.projectNumber, scope.viewNumber)
		return viewRefreshMsg{scope: scope, view: &view, err: err}
	}
}

func (m Model) updateViewRefresh(msg viewRefreshMsg) (tea.Model, tea.Cmd) {
	if !m.loadingDetail || !msg.scope.matches(m) {
		return m, nil
	}
	m.loadingDetail, m.itemsLoading = false, false
	err := msg.err
	if err == nil && (msg.view == nil || msg.view.ProjectID != msg.scope.projectID || msg.view.Number != msg.scope.viewNumber || (m.view.ID != "" && msg.view.ID != m.view.ID)) {
		err = fmt.Errorf("view refresh returned a different or missing view")
	}
	if err != nil {
		m.itemsErr = err
		m.status = "View refresh failed: " + err.Error() + "; press r to retry"
		return m, nil
	}
	m.updateViewSummary(*msg.view)
	if compatibility := m.viewCompatibility(*msg.view); !compatibility.supported() {
		m.pendingMutationReload, m.pendingIssueReload = nil, nil
		m.rejectView(*msg.view, compatibility)
		return m, nil
	}
	focusID := ""
	if item, ok := m.selectedItem(); ok {
		focusID = item.ID
	}
	m.view = msg.view
	m.tableMove = nil
	m.status = ""
	cmd := m.startItemsLoadWithSpinner()
	m.refreshFocusID = focusID
	return m, cmd
}

func (m *Model) updateViewSummary(view github.View) {
	summary := github.ViewSummary{ID: view.ID, Number: view.Number, Name: view.Name, Layout: view.Layout}
	for i := range m.views {
		if m.views[i].Number == view.Number {
			m.views[i] = summary
		}
	}
	key := viewsCacheKey(*m.selectedOwner, m.selectedProject.Number)
	if entry, ok := m.viewsCache[key]; ok {
		entry.views = append([]github.ViewSummary(nil), entry.views...)
		for i := range entry.views {
			if entry.views[i].Number == view.Number {
				entry.views[i] = summary
			}
		}
		m.viewsCache[key] = entry
	}
}
