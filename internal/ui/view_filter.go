package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type viewFilterSource interface {
	SaveViewFilter(context.Context, string, string, string) error
}
type viewFilterState struct {
	cursor                   int
	original, draft          string
	editing, preview, saving bool
}
type viewFilterSavedMsg struct {
	state *viewFilterState
	err   error
}

func (m *Model) beginViewFilterEdit() {
	if m.isTimeline() || m.view == nil || m.detailVisible {
		return
	}
	if m.itemsLoading || m.loadingDetail || m.mutationLoading || m.mutationSession != nil || m.issueActionPending != nil || m.tableAction != nil || m.actionRunning {
		m.status = "Finish pending reads and writes before editing the view"
		return
	}
	if m.viewFilter == nil {
		m.viewFilter = &viewFilterState{original: m.view.Filter}
	}
	m.viewFilter.draft, m.viewFilter.editing = m.view.Filter, true
	m.viewFilter.cursor = len([]rune(m.view.Filter))
	m.status = ""
}

func (m Model) updateViewFilterKey(key string) (tea.Model, tea.Cmd) {
	state := m.viewFilter
	if state.saving {
		return m, nil
	}
	if state.editing {
		switch key {
		case "esc":
			state.editing = false
			if !state.preview {
				m.viewFilter = nil
			}
			m.status = ""
		case "left":
			state.cursor = max(0, state.cursor-1)
		case "right":
			state.cursor = min(len([]rune(state.draft)), state.cursor+1)
		case "home", "ctrl+a":
			state.cursor = 0
		case "end", "ctrl+e":
			state.cursor = len([]rune(state.draft))
		case "backspace":
			runes := []rune(state.draft)
			if state.cursor > 0 {
				state.draft = string(append(runes[:state.cursor-1], runes[state.cursor:]...))
				state.cursor--
			}
		case "delete":
			runes := []rune(state.draft)
			if state.cursor < len(runes) {
				state.draft = string(append(runes[:state.cursor], runes[state.cursor+1:]...))
			}
		case "ctrl+u":
			state.draft = ""
			state.cursor = 0
		case "enter":
			state.editing, state.preview = false, true
			view := *m.view
			view.Filter = state.draft
			m.view = &view
			m.filter, m.filtering = "", false
			m.status = ""
			return m, m.startItemsLoadPreservingFocus()
		}
		return m, nil
	}
	if key == "ctrl+g" {
		if m.mutationLoading || m.mutationSession != nil || m.issueActionPending != nil || m.tableAction != nil || m.actionRunning {
			m.status = "Finish pending changes before discarding the preview"
			return m, nil
		}
		view := *m.view
		view.Filter = state.original
		m.view = &view
		m.viewFilter = nil
		m.status = ""
		return m, m.startItemsLoadPreservingFocus()
	}
	if key != "ctrl+s" {
		return m, nil
	}
	if m.itemsLoading || m.itemsErr != nil || m.loadingDetail || m.mutationLoading || m.mutationSession != nil || m.issueActionPending != nil || m.tableAction != nil || m.actionRunning {
		m.status = "Finish a successful preview and pending changes before saving"
		return m, nil
	}
	source, ok := m.source.(viewFilterSource)
	if !ok || !m.view.ViewerCanUpdate || m.view.ID == "" {
		m.status = "Saving this view is unavailable or not permitted"
		return m, nil
	}
	state.saving = true
	id, original, draft := m.view.ID, state.original, m.view.Filter
	m.status = ""
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
		defer cancel()
		return viewFilterSavedMsg{state: state, err: source.SaveViewFilter(ctx, id, original, draft)}
	}
}

func (m Model) updateViewFilterSaved(msg viewFilterSavedMsg) (tea.Model, tea.Cmd) {
	if m.viewFilter != msg.state {
		return m, nil
	}
	msg.state.saving = false
	if msg.err != nil {
		m.status = fmt.Sprintf("Filter save failed: %v", msg.err)
		return m, nil
	}
	m.viewFilter = nil
	if invalidator, ok := m.source.(interface{ InvalidateReads() }); ok {
		invalidator.InvalidateReads()
	}
	if m.selectedOwner != nil && m.selectedProject != nil {
		delete(m.viewsCache, viewsCacheKey(*m.selectedOwner, m.selectedProject.Number))
	}
	m.status = ""
	return m, nil
}

func (m Model) renderViewFilterEditor() string {
	return sectionStyle.Render("Edit view filter") + "\n\n" + string([]rune(m.viewFilter.draft)[:m.viewFilter.cursor]) + "▏" + string([]rune(m.viewFilter.draft)[m.viewFilter.cursor:]) + "\n\n" + mutedStyle.Render("enter preview · ctrl+u clear · esc cancel")
}

func (state *viewFilterState) insert(text string) {
	runes := []rune(state.draft)
	value := []rune(singleLineInput.Replace(text))
	state.draft = string(runes[:state.cursor]) + string(value) + string(runes[state.cursor:])
	state.cursor += len(value)
}
