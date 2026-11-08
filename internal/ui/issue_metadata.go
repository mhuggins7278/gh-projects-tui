package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type issueMetadataSource interface {
	ReadIssueMetadata(context.Context, string, github.IssueMetadataKind, bool) (github.IssueMetadata, error)
	ChangeIssueMetadata(context.Context, string, github.IssueMetadataKind, bool, []string) error
}

type assigneeSearchSource interface {
	SearchIssueAssignees(context.Context, string, string, string) (github.AssigneeSearchPage, error)
}

type issueMetadataEditor struct {
	id                uint64
	scope             boardReadScope
	issueID, itemID   string
	kind              github.IssueMetadataKind
	loading           bool
	options           []github.IssueMetadataOption
	initial, selected map[string]bool
	index             int
	searching         bool
	filter            string
	known             map[string]github.IssueMetadataOption
	searchRevision    uint64
	searchLoading     bool
	searchError       error
	searchCursor      string
	searchHasNext     bool
	searchSeen        map[string]bool
	searchCancel      context.CancelFunc
}

type metadataPending struct {
	kind        github.IssueMetadataKind
	add, remove []string // changes submitted so far, including an uncertain attempt
	err         error
	incomplete  bool
}

type metadataLoadedMsg struct {
	id   uint64
	data github.IssueMetadata
	err  error
}
type assigneeSearchDebounceMsg struct{ editorID, revision uint64 }
type assigneeSearchMsg struct {
	editorID, revision uint64
	after              string
	page               github.AssigneeSearchPage
	err                error
}
type metadataWriteMsg struct {
	actionID    uint64
	add, remove []string
	err         error
	incomplete  bool
}
type metadataReadbackMsg struct {
	actionID, readID uint64
	data             github.IssueMetadata
	err              error
}

func (m *Model) clearIssueMetadataEditor() {
	if m.issueMetadataEditor != nil && m.issueMetadataEditor.searchCancel != nil {
		m.issueMetadataEditor.searchCancel()
	}
	m.issueMetadataEditor = nil
}

func (m *Model) beginIssueMetadataEdit(kind github.IssueMetadataKind) tea.Cmd {
	if m.isTimeline() || m.detail == nil || m.detail.Content == nil || m.detail.Content.Kind != "Issue" || m.detail.Content.ID == "" {
		m.status = "This action is available for issues only"
		return nil
	}
	if m.detailLoading || m.detailErr != nil || m.itemsLoading || m.mutationLoading || m.mutationSession != nil || m.tableAction != nil || m.issueActionPending != nil {
		m.status = "Wait for pending reads and writes to finish"
		return nil
	}
	source, ok := m.source.(issueMetadataSource)
	if !ok {
		m.status = "Issue metadata editing is unavailable for this client"
		return nil
	}
	if kind == github.IssueAssignees {
		if _, ok := m.source.(assigneeSearchSource); !ok {
			m.status = "Assignee search is unavailable for this client"
			return nil
		}
	}
	m.nextIssueAction++
	editor := &issueMetadataEditor{id: m.nextIssueAction, scope: m.currentBoardReadScope(), issueID: m.detail.Content.ID, itemID: m.detailItemID, kind: kind, loading: true}
	m.issueMetadataEditor = editor
	m.status = "Loading " + string(kind) + "..."
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
		defer cancel()
		data, err := source.ReadIssueMetadata(ctx, editor.issueID, editor.kind, editor.kind != github.IssueAssignees)
		return metadataLoadedMsg{id: editor.id, data: data, err: err}
	}
}

func (m Model) updateMetadataLoaded(msg metadataLoadedMsg) (tea.Model, tea.Cmd) {
	editor := m.issueMetadataEditor
	if editor == nil || editor.id != msg.id || !editor.scope.matches(m) || !m.detailVisible || m.detailItemID != editor.itemID {
		return m, nil
	}
	if msg.err != nil {
		m.issueMetadataEditor = nil
		m.status = "Could not load " + string(editor.kind) + ": " + msg.err.Error()
		return m, nil
	}
	if !msg.data.CanEdit {
		m.issueMetadataEditor = nil
		m.status = "You do not have permission to edit " + string(editor.kind)
		return m, nil
	}
	editor.loading = false
	editor.options = msg.data.Candidates
	editor.initial, editor.selected = map[string]bool{}, map[string]bool{}
	editor.known = map[string]github.IssueMetadataOption{}
	for _, option := range msg.data.Current {
		editor.initial[option.ID], editor.selected[option.ID] = true, true
		editor.known[option.ID] = option
	}
	for _, option := range msg.data.Candidates {
		editor.known[option.ID] = option
	}
	if editor.kind == github.IssueAssignees {
		editor.searching = true
		m.status = "Type to search · enter selects results · space toggles"
		return m, m.scheduleAssigneeSearch()
	}
	m.status = "Space toggles · / searches · enter saves · esc cancels"
	return m, nil
}

func (editor *issueMetadataEditor) visibleOptions() []github.IssueMetadataOption {
	var options []github.IssueMetadataOption
	filter := strings.ToLower(strings.TrimSpace(editor.filter))
	if editor.kind == github.IssueAssignees {
		seen := map[string]bool{}
		// Keep assigned and newly selected users reachable, even when GitHub no
		// longer returns them as assignable candidates.
		var pinned []github.IssueMetadataOption
		for id, selected := range editor.selected {
			option, ok := editor.known[id]
			if (selected || editor.initial[id]) && ok && (filter == "" || strings.Contains(strings.ToLower(option.Name), filter)) {
				pinned = append(pinned, option)
			}
		}
		sort.Slice(pinned, func(i, j int) bool { return strings.ToLower(pinned[i].Name) < strings.ToLower(pinned[j].Name) })
		for _, option := range append(pinned, editor.options...) {
			if !seen[option.ID] {
				seen[option.ID] = true
				options = append(options, option)
			}
		}
		return options
	}
	for _, option := range editor.options {
		if filter == "" || strings.Contains(strings.ToLower(option.Name), filter) {
			options = append(options, option)
		}
	}
	return options
}

func (m *Model) scheduleAssigneeSearch() tea.Cmd {
	editor := m.issueMetadataEditor
	if editor == nil || editor.kind != github.IssueAssignees {
		return nil
	}
	if editor.searchCancel != nil {
		editor.searchCancel()
		editor.searchCancel = nil
	}
	editor.searchRevision++
	editor.searchLoading = true
	editor.searchError = nil
	editor.searchCursor, editor.searchHasNext = "", false
	editor.searchSeen = map[string]bool{}
	editor.options = nil
	editor.index = 0
	id, revision := editor.id, editor.searchRevision
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return assigneeSearchDebounceMsg{editorID: id, revision: revision} })
}

func (m *Model) startAssigneeSearch(after string) tea.Cmd {
	editor := m.issueMetadataEditor
	if editor == nil || editor.kind != github.IssueAssignees {
		return nil
	}
	source, ok := m.source.(assigneeSearchSource)
	if !ok {
		editor.searchLoading = false
		editor.searchError = fmt.Errorf("assignee search is unavailable")
		return nil
	}
	if editor.searchCancel != nil {
		editor.searchCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
	editor.searchCancel = cancel
	editor.searchLoading = true
	editor.searchError = nil
	id, revision, issueID, query := editor.id, editor.searchRevision, editor.issueID, editor.filter
	return func() tea.Msg {
		defer cancel()
		page, err := source.SearchIssueAssignees(ctx, issueID, query, after)
		return assigneeSearchMsg{editorID: id, revision: revision, after: after, page: page, err: err}
	}
}

func (m Model) updateAssigneeSearch(msg assigneeSearchMsg) (tea.Model, tea.Cmd) {
	editor := m.issueMetadataEditor
	if editor == nil || editor.id != msg.editorID || editor.searchRevision != msg.revision || !editor.scope.matches(m) || !m.detailVisible || m.detailItemID != editor.itemID {
		return m, nil
	}
	editor.searchCancel = nil
	editor.searchLoading = false
	if msg.err != nil {
		editor.searchError = msg.err
		return m, nil
	}
	if !msg.page.CanEdit {
		m.issueMetadataEditor = nil
		m.status = "You do not have permission to edit assignees"
		return m, nil
	}
	if msg.page.HasNext && (msg.page.EndCursor == "" || editor.searchSeen[msg.page.EndCursor]) {
		editor.searchError = fmt.Errorf("assignee search pagination did not advance")
		editor.searchHasNext = false
		return m, nil
	}
	if msg.after == "" {
		editor.options = nil
	}
	seen := map[string]bool{}
	for _, option := range editor.options {
		seen[option.ID] = true
	}
	for _, option := range msg.page.Options {
		editor.known[option.ID] = option
		if !seen[option.ID] {
			editor.options = append(editor.options, option)
			seen[option.ID] = true
		}
	}
	editor.searchCursor, editor.searchHasNext = msg.page.EndCursor, msg.page.HasNext
	if msg.page.HasNext {
		editor.searchSeen[msg.page.EndCursor] = true
	}
	return m, nil
}

func (m Model) updateMetadataEditorKey(key string) (tea.Model, tea.Cmd) {
	editor := m.issueMetadataEditor
	if key == "esc" {
		if editor.searching {
			editor.searching = false
			return m, nil
		}
		if editor.searchCancel != nil {
			editor.searchCancel()
		}
		m.issueMetadataEditor = nil
		m.status = "Edit cancelled"
		return m, nil
	}
	if editor.loading {
		return m, nil
	}
	if editor.searching {
		switch key {
		case "enter":
			editor.searching = false
		case "backspace":
			editor.filter = deleteLastRune(editor.filter)
			editor.index = 0
			if editor.kind == github.IssueAssignees {
				return m, m.scheduleAssigneeSearch()
			}
		case "ctrl+u":
			editor.filter = ""
			editor.index = 0
			if editor.kind == github.IssueAssignees {
				return m, m.scheduleAssigneeSearch()
			}
		}
		return m, nil
	}
	options := editor.visibleOptions()
	switch key {
	case "/":
		editor.searching = true
	case "j", "down":
		if editor.index+1 < len(options) {
			editor.index++
		} else if editor.kind == github.IssueAssignees && editor.searchHasNext && !editor.searchLoading {
			return m, m.startAssigneeSearch(editor.searchCursor)
		}
	case "k", "up":
		if editor.index > 0 {
			editor.index--
		}
	case "space", " ":
		if editor.index < len(options) {
			id := options[editor.index].ID
			editor.selected[id] = !editor.selected[id]
			if editor.kind == github.IssueAssignees {
				// Toggling a user can move them into or out of the pinned rows.
				options = editor.visibleOptions()
				editor.index = min(editor.index, max(0, len(options)-1))
				for i, option := range options {
					if option.ID == id {
						editor.index = i
						break
					}
				}
			}
		}
	case "enter":
		if !editor.scope.matches(m) || !m.detailVisible || m.detailItemID != editor.itemID {
			m.issueMetadataEditor = nil
			m.status = "View changed; select the issue again"
			return m, nil
		}
		var add, remove []string
		for id := range editor.known {
			if editor.selected[id] && !editor.initial[id] {
				add = append(add, id)
			}
			if editor.initial[id] && !editor.selected[id] {
				remove = append(remove, id)
			}
		}
		sort.Strings(add)
		sort.Strings(remove)
		if editor.searchCancel != nil {
			editor.searchCancel()
		}
		m.issueMetadataEditor = nil
		if len(add) == 0 && len(remove) == 0 {
			m.status = "No changes"
			return m, nil
		}
		source, ok := m.source.(issueMetadataSource)
		if !ok {
			m.status = "Issue metadata editing is unavailable"
			return m, nil
		}
		m.nextIssueAction++
		pending := &issueActionState{id: m.nextIssueAction, issueID: editor.issueID, itemID: editor.itemID, scope: editor.scope, metadata: &metadataPending{kind: editor.kind}}
		m.issueActionPending = pending
		m.mutationLoading = true
		m.status = "Saving " + string(editor.kind) + "..."
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
			defer cancel()
			result := metadataWriteMsg{actionID: pending.id}
			if len(add) > 0 {
				err := source.ChangeIssueMetadata(ctx, editor.issueID, editor.kind, true, add)
				if err == nil || github.IsAmbiguousMutationError(err) {
					result.add = add
				}
				if err != nil {
					result.err = err
					result.incomplete = !github.IsAmbiguousMutationError(err) || len(remove) > 0
					return result
				}
			}
			if len(remove) > 0 {
				err := source.ChangeIssueMetadata(ctx, editor.issueID, editor.kind, false, remove)
				if err == nil || github.IsAmbiguousMutationError(err) {
					result.remove = remove
				}
				if err != nil {
					result.err = err
					result.incomplete = !github.IsAmbiguousMutationError(err)
				}
			}
			return result
		}
	}
	return m, nil
}

func (m Model) renderIssueMetadataEditor() string {
	editor := m.issueMetadataEditor
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", sectionStyle.Render("Edit "+string(editor.kind)))
	if editor.loading {
		b.WriteString("Loading options...\n")
		return b.String()
	}
	search := editor.filter
	if editor.searching {
		search += "_"
	}
	fmt.Fprintf(&b, "Search: %s\n", search)
	if editor.kind == github.IssueAssignees && editor.searchError != nil {
		b.WriteString(truncateText("Search failed: "+editor.searchError.Error(), m.detailPanelWidth()-4) + "\n")
	}
	options := editor.visibleOptions()
	if len(options) == 0 && !editor.searchLoading && editor.searchError == nil {
		b.WriteString("No matching options\n")
	}
	start, end := m.tableWindow(len(options), editor.index)
	for index := start; index < end; index++ {
		marker := "  "
		if index == editor.index {
			marker = "> "
		}
		check := "[ ] "
		if editor.selected[options[index].ID] {
			check = "[x] "
		}
		b.WriteString(truncateText(marker+check+options[index].Name, m.detailPanelWidth()-4) + "\n")
	}
	if editor.kind == github.IssueAssignees && editor.searchLoading {
		b.WriteString(mutedStyle.Render("Searching...\n"))
	}
	if editor.kind == github.IssueAssignees && editor.searchHasNext {
		b.WriteString(mutedStyle.Render("↓ at end loads more\n"))
	}
	if editor.searching {
		b.WriteString(mutedStyle.Render("type search · enter choose · esc stop searching"))
	} else {
		b.WriteString(mutedStyle.Render("/ search · space toggle · enter save · esc cancel"))
	}
	return b.String()
}

func (m Model) updateMetadataWrite(msg metadataWriteMsg) (tea.Model, tea.Cmd) {
	pending := m.issueActionPending
	if pending == nil || pending.id != msg.actionID || pending.metadata == nil {
		return m, nil
	}
	pending.metadata.add, pending.metadata.remove = msg.add, msg.remove
	pending.metadata.err, pending.metadata.incomplete = msg.err, msg.incomplete
	// Even a definite rejection may follow a successful first step. Read back
	// before unlocking so the UI and filtered membership reflect GitHub.
	m.status = "Checking issue " + string(pending.metadata.kind) + "..."
	return m, m.startIssueReadback()
}

func (m *Model) startMetadataReadback(pending *issueActionState) tea.Cmd {
	pending.readID++
	actionID, readID, issueID, kind := pending.id, pending.readID, pending.issueID, pending.metadata.kind
	source, ok := m.source.(issueMetadataSource)
	m.issueActionRefreshing = true
	return func() tea.Msg {
		if !ok {
			return metadataReadbackMsg{actionID: actionID, readID: readID, err: fmt.Errorf("issue metadata readback unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
		defer cancel()
		data, err := source.ReadIssueMetadata(ctx, issueID, kind, false)
		return metadataReadbackMsg{actionID: actionID, readID: readID, data: data, err: err}
	}
}

func (m Model) updateMetadataReadback(msg metadataReadbackMsg) (tea.Model, tea.Cmd) {
	pending := m.issueActionPending
	if pending == nil || pending.metadata == nil || pending.id != msg.actionID || pending.readID != msg.readID {
		return m, nil
	}
	m.issueActionRefreshing = false
	state := map[string]bool{}
	for _, option := range msg.data.Current {
		state[option.ID] = true
	}
	confirmed := msg.err == nil
	for _, id := range pending.metadata.add {
		if !state[id] {
			confirmed = false
		}
	}
	for _, id := range pending.metadata.remove {
		if state[id] {
			confirmed = false
		}
	}
	if !confirmed {
		pending.blocked = true
		m.mutationLoading = true
		m.status = "Issue outcome unknown; press r to check again, esc returns to the board. Further writes are blocked."
		return m, nil
	}
	delete(m.detailCache, pending.itemID)
	if invalidator, ok := m.source.(interface{ InvalidateReads() }); ok {
		invalidator.InvalidateReads()
	}
	m.issueActionPending = nil
	m.mutationLoading = false
	if pending.metadata.incomplete {
		m.status = "Some changes may not have been saved: " + pending.metadata.err.Error()
	} else if pending.metadata.err != nil {
		m.status = "Issue " + string(pending.metadata.kind) + " reconciled"
	} else {
		m.status = "Issue " + string(pending.metadata.kind) + " saved"
	}
	if !pending.scope.sameProject(m) {
		return m, nil
	}
	if m.screen == screenBoard && m.view != nil {
		scope := m.currentBoardReadScope()
		m.pendingIssueReload = &scope
	}
	if m.detailVisible && m.detailItemID == pending.itemID && pending.scope.matches(m) {
		m.detailLoading = true
		m.detailErr = nil
		m.detailRequestID++
		return m, m.loadItemDetailCmd(pending.itemID, m.detailRequestID, m.currentBoardReadScope())
	}
	return m, m.finishIssueViewReload()
}
