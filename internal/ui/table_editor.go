package ui

import (
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
	scope       boardReadScope
	itemID      string
	fields      []github.Field
	fieldIndex  int
	choosing    bool
	draft       string
	optionIndex int
	selected    map[string]bool
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

func (m *Model) beginTableFieldEdit() {
	if reason := m.tableActionUnavailable(); reason != "" {
		m.status = reason
		return
	}
	if m.loadingDetail {
		m.status = "Wait for view settings to finish refreshing"
		return
	}
	if _, ok := m.source.(ItemMutationSource); !ok {
		m.status = "Field editing is unavailable for this client"
		return
	}
	item, ok := m.selectedItem()
	if !ok {
		return
	}
	fields := []github.Field{}
	for _, field := range m.view.Fields {
		if !editableTableField(field) {
			continue
		}
		if field.IsIssueField && (item.Content == nil || item.Content.Kind != "Issue" || item.Content.ID == "" || (item.Content.ViewerCanSetFields != nil && !*item.Content.ViewerCanSetFields)) {
			continue
		}
		fields = append(fields, field)
	}
	if len(fields) == 0 {
		m.status = "No editable saved columns for this item"
		return
	}
	m.tableEditor = &tableFieldEditor{scope: m.currentBoardReadScope(), itemID: item.ID, fields: fields, choosing: true}
	m.status = "Choose a field with j/k; enter edits, esc cancels"
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
	if editor == nil || editor.choosing {
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
	if reason := m.tableActionUnavailable(); reason != "" {
		m.status = reason
		return m, nil
	}
	m.tableEditor = nil
	return m, m.enqueueBoardMutation(boardMutationIntent{kind: boardMutationSetField, itemID: editor.itemID, field: field, value: value, clear: clear, description: "Edit " + field.Name})
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
	if editor.choosing {
		var b strings.Builder
		b.WriteString(sectionStyle.Render("Edit saved column") + "\n")
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
