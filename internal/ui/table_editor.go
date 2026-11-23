package ui

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type tableFieldEditor struct {
	filter      string
	detail      bool
	loading     bool
	requestID   uint64
	values      []github.DetailField
	scope       boardReadScope
	itemID      string
	fields      []github.Field
	fieldIndex  int
	choosing    bool
	draft       string
	optionIndex int
	selected    map[string]bool
}

type selectedFieldsLoadedMsg struct {
	scope     boardReadScope
	itemID    string
	requestID uint64
	detail    github.ItemDetail
	err       error
}

func (m *Model) beginSelectedFieldEdit() tea.Cmd {
	if reason := m.tableActionUnavailable(); reason != "" {
		m.status = reason
		return nil
	}
	if m.loadingDetail {
		m.status = "Wait for view settings to finish refreshing"
		return nil
	}
	item, ok := m.selectedItem()
	if !ok || item.ID == "" {
		m.status = "Select an item first"
		return nil
	}
	loader, ok := m.source.(ItemDetailSource)
	if !ok {
		m.status = "Field editing is unavailable for this client"
		return nil
	}
	if _, ok := m.source.(ItemMutationSource); !ok {
		m.status = "Field editing is unavailable for this client"
		return nil
	}
	scope, owner, project := m.currentBoardReadScope(), *m.selectedOwner, m.selectedProject.Number
	source := m.source
	m.nextFieldRequest++
	requestID := m.nextFieldRequest
	m.tableEditor = &tableFieldEditor{scope: scope, itemID: item.ID, requestID: requestID, loading: true, choosing: true}
	m.status = ""
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
		defer cancel()
		var detail github.ItemDetail
		var err error
		if fieldsLoader, ok := source.(interface {
			LoadItemFields(context.Context, github.Owner, int, string) (github.ItemDetail, error)
		}); ok {
			detail, err = fieldsLoader.LoadItemFields(ctx, owner, project, item.ID)
		} else {
			detail, err = loader.LoadItemDetail(ctx, owner, project, item.ID)
		}
		return selectedFieldsLoadedMsg{scope: scope, itemID: item.ID, requestID: requestID, detail: detail, err: err}
	}
}

func (m Model) updateSelectedFieldsLoaded(msg selectedFieldsLoadedMsg) (tea.Model, tea.Cmd) {
	editor := m.tableEditor
	if editor == nil || !editor.loading || editor.scope != msg.scope || editor.itemID != msg.itemID || editor.requestID != msg.requestID || !msg.scope.matches(m) {
		return m, nil
	}
	item, ok := m.selectedItem()
	if !ok || item.ID != editor.itemID {
		m.tableEditor = nil
		return m, nil
	}
	if msg.err != nil {
		m.tableEditor = nil
		m.status = "Could not load project fields: " + msg.err.Error()
		return m, nil
	}
	if msg.detail.ID != msg.itemID {
		m.tableEditor = nil
		m.status = "Selected item details are unavailable; refresh and try again"
		return m, nil
	}
	for _, detailField := range msg.detail.Fields {
		field := detailField.Field
		if !editableTableField(field) || (detailField.Value != nil && !detailField.Value.Available) {
			continue
		}
		if field.IsIssueField && (msg.detail.Content == nil || msg.detail.Content.Kind != "Issue" || msg.detail.Content.ID == "" || (msg.detail.Content.ViewerCanSetFields != nil && !*msg.detail.Content.ViewerCanSetFields)) {
			continue
		}
		editor.fields = append(editor.fields, field)
	}
	if len(editor.fields) == 0 {
		m.tableEditor = nil
		m.status = "No editable project fields for this item"
		return m, nil
	}
	editor.values = msg.detail.Fields
	editor.loading = false
	m.status = ""
	return m, nil
}

func (m *Model) beginDetailFieldEdit() {
	if m.isTimeline() || m.view == nil || m.view.ProjectID == "" || m.selectedOwner == nil || m.selectedProject == nil || !m.view.ViewerCanUpdate || m.detail == nil || m.detail.Content == nil || m.detail.Content.Kind != "Issue" {
		m.status = "Project field editing is unavailable for this item"
		return
	}
	if m.detailLoading || m.loadingDetail || m.itemsLoading || m.itemsErr != nil || m.mutationLoading || m.mutationSession != nil || m.tableAction != nil || m.issueActionPending != nil {
		m.status = "Wait for pending reads and writes to finish"
		return
	}
	if m.detailErr != nil {
		m.status = "Reload issue details before editing fields"
		return
	}
	if strings.TrimSpace(m.filter) != "" || m.filtering {
		m.status = "Clear local search before changing project items"
		return
	}
	item, ok := m.selectedItem()
	if !ok || item.ID != m.detailItemID || item.OutsideProject {
		m.status = "This issue is not an active item in this project"
		return
	}
	if _, ok := m.source.(ItemMutationSource); !ok {
		m.status = "Field editing is unavailable for this client"
		return
	}
	fields := []github.Field{}
	for _, detailField := range m.detail.Fields {
		field := detailField.Field
		if editableTableField(field) && (detailField.Value == nil || detailField.Value.Available) && (!field.IsIssueField || (m.detail.Content.ID != "" && (m.detail.Content.ViewerCanSetFields == nil || *m.detail.Content.ViewerCanSetFields))) {
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		m.status = "No editable project fields for this issue"
		return
	}
	m.tableEditor = &tableFieldEditor{detail: true, scope: m.currentBoardReadScope(), itemID: item.ID, fields: fields, choosing: true}
	m.status = ""
}

func editableTableField(field github.Field) bool {
	if field.ID == "" || isTitleField(field) {
		return false
	}
	if field.IsIssueField && field.IssueFieldID == "" {
		return false
	}
	switch strings.ToUpper(field.DataType) {
	case "TEXT", "NUMBER", "DATE":
		return true
	case "SINGLE_SELECT", "MULTI_SELECT":
		return len(field.Options) > 0
	case "ITERATION":
		return !field.IsIssueField && len(field.Iterations) > 0
	}
	return false
}

func (m *Model) chooseTableField() {
	editor := m.tableEditor
	field := editor.fields[editor.fieldIndex]
	editor.choosing = false
	editor.selected = map[string]bool{}
	item, ok := m.selectedItem()
	if !ok || item.ID != editor.itemID {
		m.tableEditor = nil
		return
	}
	value, found := itemFieldValue(field, item)
	if editor.values != nil {
		found = false
		for _, detailField := range editor.values {
			if detailField.Field.ID == field.ID && detailField.Value != nil {
				value, found = *detailField.Value, true
				break
			}
		}
	} else if editor.detail && m.detail != nil {
		found = false
		for _, detailField := range m.detail.Fields {
			if detailField.Field.ID == field.ID && detailField.Value != nil {
				value, found = *detailField.Value, true
				break
			}
		}
	}
	if found && value.Available {
		editor.draft = value.Value
		for _, option := range value.Options {
			editor.selected[option.ID] = true
		}
		if value.OptionID != "" {
			editor.selected[value.OptionID] = true
		}
		if value.IterationID != "" {
			editor.selected[value.IterationID] = true
		}
	}
	for index, option := range editorOptions(field) {
		if editor.selected[option.ID] {
			editor.optionIndex = index + 1
			break
		}
	}
	m.status = ""
}

func editorOptions(field github.Field) []github.FieldOption {
	if !isIterationField(field) {
		return field.Options
	}
	options := make([]github.FieldOption, 0, len(field.Iterations))
	for _, iteration := range field.Iterations {
		options = append(options, github.FieldOption{ID: iteration.ID, Name: iteration.Title})
	}
	return options
}

func (editor *tableFieldEditor) acceptsText() bool {
	if editor == nil || editor.loading || editor.choosing {
		return false
	}
	field := editor.fields[editor.fieldIndex]
	return !isSingleSelectField(field) && !isMultiSelectField(field) && !isIterationField(field)
}

func (m Model) updateTableEditorKey(key string) (tea.Model, tea.Cmd) {
	editor := m.tableEditor
	if key == "esc" {
		m.tableEditor = nil
		m.status = ""
		return m, nil
	}
	if editor.loading {
		return m, nil
	}
	if !editor.acceptsText() {
		if key == "backspace" || key == "ctrl+u" {
			if key == "ctrl+u" {
				editor.filter = ""
			} else {
				editor.filter = deleteLastRune(editor.filter)
			}
			editor.selectFirstMatch()
			return m, nil
		}
		if key == "down" || key == "up" || key == "ctrl+n" || key == "ctrl+p" {
			indices := editor.matchingIndices()
			current := editor.optionIndex
			if editor.choosing {
				current = editor.fieldIndex
			}
			for i, index := range indices {
				if index == current {
					if (key == "down" || key == "ctrl+n") && i+1 < len(indices) {
						current = indices[i+1]
					}
					if (key == "up" || key == "ctrl+p") && i > 0 {
						current = indices[i-1]
					}
					break
				}
			}
			if editor.choosing {
				editor.fieldIndex = current
			} else {
				editor.optionIndex = current
			}
			return m, nil
		}
		if len(editor.matchingIndices()) == 0 {
			return m, nil
		}
	}
	if editor.choosing {
		if key == "enter" {
			m.chooseTableField()
			editor.filter = ""
		}
		return m, nil
	}
	field := editor.fields[editor.fieldIndex]
	options := editorOptions(field)
	if editor.acceptsText() {
		if key == "backspace" {
			editor.draft = deleteLastRune(editor.draft)
		}
		if key == "ctrl+u" {
			editor.draft = ""
		}
	} else {
		switch key {
		case "j", "down":
			if editor.optionIndex < len(options) {
				editor.optionIndex++
			}
		case "k", "up":
			if editor.optionIndex > 0 {
				editor.optionIndex--
			}
		case "space", " ":
			if isMultiSelectField(field) {
				if editor.optionIndex == 0 {
					editor.selected = map[string]bool{}
				} else {
					id := options[editor.optionIndex-1].ID
					editor.selected[id] = !editor.selected[id]
				}
			}
		}
	}
	if key != "enter" {
		return m, nil
	}
	value, clear, err := editor.value()
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	if !editor.scope.matches(m) {
		m.tableEditor = nil
		m.status = "View changed; select the field again"
		return m, nil
	}
	if !editor.detail {
		if reason := m.tableActionUnavailable(); reason != "" {
			m.status = reason
			return m, nil
		}
	}
	m.tableEditor = nil
	return m, m.enqueueBoardMutation(boardMutationIntent{kind: boardMutationSetField, itemID: editor.itemID, field: field, value: value, clear: clear, forceFieldWrite: editor.detail, description: "Edit " + field.Name})
}

func (editor *tableFieldEditor) value() (github.FieldValueInput, bool, error) {
	field := editor.fields[editor.fieldIndex]
	if isMultiSelectField(field) {
		ids := []string{}
		for _, option := range field.Options {
			if editor.selected[option.ID] {
				ids = append(ids, option.ID)
			}
		}
		return github.FieldValueInput{MultiSelectOptionIDs: ids}, len(ids) == 0, nil
	}
	if isSingleSelectField(field) || isIterationField(field) {
		if editor.optionIndex == 0 {
			return github.FieldValueInput{}, true, nil
		}
		options := editorOptions(field)
		if editor.optionIndex > len(options) {
			return github.FieldValueInput{}, false, fmt.Errorf("Option is no longer available")
		}
		id := options[editor.optionIndex-1].ID
		if isIterationField(field) {
			return github.FieldValueInput{IterationID: &id}, false, nil
		}
		return github.FieldValueInput{SingleSelectOptionID: &id}, false, nil
	}
	text := editor.draft
	if strings.TrimSpace(text) == "" {
		return github.FieldValueInput{}, true, nil
	}
	switch strings.ToUpper(field.DataType) {
	case "TEXT":
		return github.FieldValueInput{Text: &text}, false, nil
	case "NUMBER":
		number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return github.FieldValueInput{}, false, fmt.Errorf("Enter a finite number")
		}
		return github.FieldValueInput{Number: &number}, false, nil
	case "DATE":
		text = strings.TrimSpace(text)
		if _, err := time.Parse("2006-01-02", text); err != nil {
			return github.FieldValueInput{}, false, fmt.Errorf("Enter a valid date as YYYY-MM-DD")
		}
		return github.FieldValueInput{Date: &text}, false, nil
	}
	return github.FieldValueInput{}, false, fmt.Errorf("Unsupported field type")
}

func optionIDs(options []github.FieldOption) []string {
	ids := make([]string, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	sort.Strings(ids)
	return ids
}

func (m Model) renderTableEditor() string {
	editor := m.tableEditor
	if editor.loading {
		return sectionStyle.Render("Loading project fields...")
	}
	if editor.choosing {
		var b strings.Builder
		b.WriteString(m.editorIssueContext() + "\n")
		b.WriteString(sectionStyle.Render("Edit project field") + "\n")
		b.WriteString("Search: " + editor.filter + "_\n\n")
		indices := editor.matchingIndices()
		if len(indices) == 0 {
			b.WriteString(mutedStyle.Render("No matches") + "\n")
		}
		start, end := m.editorWindow(len(indices), matchPosition(indices, editor.fieldIndex))
		for _, index := range indices[start:end] {
			marker := "  "
			if index == editor.fieldIndex {
				marker = "> "
			}
			b.WriteString(truncateText(marker+editor.fields[index].Name+"    "+m.editorFieldValue(editor, editor.fields[index]), m.detailPanelWidth()-4) + "\n")
		}
		return b.String()
	}
	field := editor.fields[editor.fieldIndex]
	var b strings.Builder
	b.WriteString(m.editorIssueContext() + "\n")
	b.WriteString(sectionStyle.Render("Edit "+field.Name) + "\n")
	if editor.acceptsText() {
		b.WriteString(truncateText(editor.draft+"_", m.detailPanelWidth()-4) + "\n")
		b.WriteString(mutedStyle.Render("ctrl+u clears · enter saves · esc cancels") + "\n")
	} else {
		options := append([]github.FieldOption{{Name: "Clear value"}}, editorOptions(field)...)
		b.WriteString("Search: " + editor.filter + "_\n\n")
		indices := editor.matchingIndices()
		if len(indices) == 0 {
			b.WriteString(mutedStyle.Render("No matches") + "\n")
		}
		start, end := m.editorWindow(len(indices), matchPosition(indices, editor.optionIndex))
		for _, index := range indices[start:end] {
			marker := "  "
			if index == editor.optionIndex {
				marker = "> "
			}
			if index > 0 && !isMultiSelectField(field) && editor.selected[options[index].ID] {
				marker += "✓ "
			}
			if index > 0 && isMultiSelectField(field) {
				if editor.selected[options[index].ID] {
					marker += "[x] "
				} else {
					marker += "[ ] "
				}
			}
			b.WriteString(truncateText(marker+options[index].Name, m.detailPanelWidth()-4) + "\n")
		}
		if isMultiSelectField(field) {
			b.WriteString(mutedStyle.Render("space toggles · enter saves selection") + "\n")
		}
	}
	return b.String()
}

func matchPosition(indices []int, current int) int {
	for i, index := range indices {
		if index == current {
			return i
		}
	}
	return 0
}
func (editor *tableFieldEditor) matchingIndices() []int {
	var names []string
	if editor.choosing {
		for _, field := range editor.fields {
			names = append(names, field.Name)
		}
	} else {
		names = append(names, "Clear value")
		for _, option := range editorOptions(editor.fields[editor.fieldIndex]) {
			names = append(names, option.Name)
		}
	}
	var indices []int
	for i, name := range names {
		if strings.Contains(strings.ToLower(name), strings.ToLower(editor.filter)) {
			indices = append(indices, i)
		}
	}
	return indices
}
func (editor *tableFieldEditor) selectFirstMatch() {
	indices := editor.matchingIndices()
	if len(indices) == 0 {
		return
	}
	if editor.choosing {
		editor.fieldIndex = indices[0]
	} else {
		editor.optionIndex = indices[0]
	}
}
func (m Model) editorFieldValue(editor *tableFieldEditor, field github.Field) string {
	values := editor.values
	if editor.detail && m.detail != nil {
		values = m.detail.Fields
	}
	for _, value := range values {
		if value.Field.ID == field.ID && value.Value != nil && value.Value.Available && value.Value.Value != "" {
			return value.Value.Value
		}
	}
	return "—"
}

func (m Model) editorIssueContext() string {
	item, ok := m.selectedItem()
	if !ok || item.Content == nil {
		return ""
	}
	return mutedStyle.Render(truncateText(fmt.Sprintf("#%d %s", item.Content.Number, item.Content.Title), m.detailPanelWidth()-4))
}

func (m Model) editorWindow(count, selected int) (int, int) {
	size := min(8, max(1, m.detailPanelHeight()-9))
	start := max(0, selected-size/2)
	start = min(start, max(0, count-size))
	return start, min(count, start+size)
}
