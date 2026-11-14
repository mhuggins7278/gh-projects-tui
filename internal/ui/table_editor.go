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
	m.status = "Loading project fields..."
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
	m.status = "Choose a project field with j/k; enter edits, esc cancels"
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
	m.status = "Choose a project field with j/k; enter edits, esc cancels"
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
	m.status = "Enter saves; esc cancels. Empty scalar values clear the field."
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
		m.status = "Field edit cancelled"
		return m, nil
	}
	if editor.loading {
		return m, nil
	}
	if editor.choosing {
		switch key {
		case "j", "down":
			if editor.fieldIndex+1 < len(editor.fields) {
				editor.fieldIndex++
			}
		case "k", "up":
			if editor.fieldIndex > 0 {
				editor.fieldIndex--
			}
		case "enter":
			m.chooseTableField()
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
		b.WriteString(sectionStyle.Render("Edit project field") + "\n")
		start, end := m.tableWindow(len(editor.fields), editor.fieldIndex)
		for index := start; index < end; index++ {
			marker := "  "
			if index == editor.fieldIndex {
				marker = "> "
			}
			b.WriteString(truncateText(marker+editor.fields[index].Name, m.frameContentWidth()-2) + "\n")
		}
		return b.String()
	}
	field := editor.fields[editor.fieldIndex]
	var b strings.Builder
	b.WriteString(sectionStyle.Render("Edit "+field.Name) + "\n")
	if editor.acceptsText() {
		b.WriteString(truncateText(editor.draft+"_", m.frameContentWidth()-2) + "\n")
		b.WriteString(mutedStyle.Render("ctrl+u clears · enter saves · esc cancels") + "\n")
	} else {
		options := append([]github.FieldOption{{Name: "Clear value"}}, editorOptions(field)...)
		start, end := m.tableWindow(len(options), editor.optionIndex)
		for index := start; index < end; index++ {
			marker := "  "
			if index == editor.optionIndex {
				marker = "> "
			}
			if index > 0 && isMultiSelectField(field) {
				if editor.selected[options[index].ID] {
					marker += "[x] "
				} else {
					marker += "[ ] "
				}
			}
			b.WriteString(truncateText(marker+options[index].Name, m.frameContentWidth()-2) + "\n")
		}
		if isMultiSelectField(field) {
			b.WriteString(mutedStyle.Render("space toggles · enter saves selection") + "\n")
		}
	}
	return b.String()
}
