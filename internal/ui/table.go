package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func (m Model) renderTableContent(b *strings.Builder) {
	if m.loadingDetail {
		b.WriteString("Loading view...\n")
		return
	}

	fmt.Fprintf(b, "%s  %s  %s\n", titleStyle.Render(m.view.Name), mutedStyle.Render(fmt.Sprintf("#%d", m.view.Number)), layoutBadge(string(m.view.Layout)))
	b.WriteString(mutedStyle.Render("Display note: rendering saved visible fields in API order.") + "\n")
	if m.view.Filter != "" {
		fmt.Fprintf(b, "%s %s\n", mutedStyle.Render("Filter:"), m.view.Filter)
	}
	visibleItems := m.boardItems()
	if m.filtering || strings.TrimSpace(m.filter) != "" {
		search := m.filter
		if m.filtering {
			search += "_"
		}
		fmt.Fprintf(b, "%s %s (%d matches)\n", statusStyle.Render("Search:"), search, len(visibleItems))
	}
	fmt.Fprintf(b, "%s %d", mutedStyle.Render("Items:"), len(visibleItems))
	if m.itemsLoading {
		b.WriteString("  " + statusStyle.Render(m.itemsLoadingSpinner()+" loading"))
	}
	b.WriteString("\n")

	if m.itemsErr != nil && len(m.items) == 0 && !m.itemsLoading {
		b.WriteString("\n" + errorStyle.Render("Items failed: "+m.itemsErr.Error()) + "\n")
		b.WriteString(mutedStyle.Render("Press r to retry.") + "\n")
		return
	}
	if len(visibleItems) == 0 {
		if m.itemsLoading {
			b.WriteString("\n" + mutedStyle.Render("Loading items...") + "\n")
		} else if strings.TrimSpace(m.filter) != "" {
			b.WriteString("\n" + mutedStyle.Render("No items match the current search.") + "\n")
		} else {
			b.WriteString("\n" + mutedStyle.Render("No issues, pull requests, or drafts match this view.") + "\n")
		}
		return
	}

	columns := tableColumns(m.view)
	widths := tableColumnWidths(columns, m.frameContentWidth()-2)
	b.WriteString("\n")
	b.WriteString(tableHeaderRow(columns, widths) + "\n")
	b.WriteString(tableRule(widths) + "\n")

	start, end := m.tableWindow(len(visibleItems))
	if start > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("... %d earlier", start)) + "\n")
	}
	for index := start; index < end; index++ {
		cells := make([]string, 0, len(columns))
		for _, column := range columns {
			cells = append(cells, tableCellValue(column, visibleItems[index]))
		}
		b.WriteString(tableDataRow(cells, widths, index == m.boardCard) + "\n")
	}
	if end < len(visibleItems) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("... %d more", len(visibleItems)-end)) + "\n")
	}
	if m.itemsErr != nil {
		b.WriteString(errorStyle.Render("Items incomplete: "+m.itemsErr.Error()) + "\n")
	}
}

func (m Model) tableWindow(count int) (int, int) {
	rows := m.boardViewportHeight() - 4
	if rows < 1 {
		rows = 1
	}
	if rows > count {
		rows = count
	}
	start := 0
	if m.boardCard >= rows {
		start = m.boardCard - rows + 1
	}
	end := start + rows
	if end > count {
		end = count
		start = end - rows
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func tableColumns(view *github.View) []github.Field {
	if view == nil {
		return nil
	}
	columns := append([]github.Field(nil), view.Fields...)
	for _, field := range columns {
		if isTitleField(field) {
			return columns
		}
	}
	return append([]github.Field{{Name: "Title", DataType: "TITLE"}}, columns...)
}

func isTitleField(field github.Field) bool {
	return strings.EqualFold(field.Name, "Title") || strings.EqualFold(field.DataType, "TITLE")
}

func tableFieldName(field github.Field) string {
	if field.Name != "" {
		return field.Name
	}
	if field.DataType != "" {
		return field.DataType
	}
	return "Field"
}

func tableColumnWidth(field github.Field, value string) int {
	width := lipgloss.Width(value)
	if labelWidth := lipgloss.Width(tableFieldName(field)); labelWidth > width {
		width = labelWidth
	}
	if width < 8 {
		width = 8
	}
	if width > 28 {
		width = 28
	}
	return width
}

func tableColumnWidths(columns []github.Field, available int) []int {
	if len(columns) == 0 {
		return nil
	}
	budget := available - 2 - (len(columns)-1)*3
	if budget < len(columns) {
		budget = len(columns)
	}
	widths := make([]int, len(columns))
	naturalTotal := 0
	for index, column := range columns {
		widths[index] = tableColumnWidth(column, "")
		naturalTotal += widths[index]
	}
	if naturalTotal > budget {
		base := budget / len(columns)
		for index := range widths {
			widths[index] = base
			if index < budget%len(columns) {
				widths[index]++
			}
		}
		return widths
	}
	for extra := budget - naturalTotal; extra > 0; extra-- {
		widths[extra%len(widths)]++
	}
	return widths
}

func tableHeaderRow(columns []github.Field, widths []int) string {
	labels := make([]string, 0, len(columns))
	for _, column := range columns {
		labels = append(labels, tableFieldName(column))
	}
	return sectionStyle.Render(tableRow(labels, widths, false))
}

func tableDataRow(cells []string, widths []int, selected bool) string {
	row := tableRow(cells, widths, selected)
	if selected {
		return selectedRowStyle.Render(row)
	}
	return rowStyle.Render(row)
}

func tableRow(cells []string, widths []int, selected bool) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	parts := make([]string, 0, len(cells))
	for index, cell := range cells {
		width := widths[index]
		cell = truncateText(strings.ReplaceAll(cell, "\n", " "), width)
		padding := width - lipgloss.Width(cell)
		if padding < 0 {
			padding = 0
		}
		parts = append(parts, cell+strings.Repeat(" ", padding))
	}
	return prefix + strings.Join(parts, " | ")
}

func tableRule(widths []int) string {
	parts := make([]string, 0, len(widths))
	for _, width := range widths {
		parts = append(parts, strings.Repeat("-", width))
	}
	return mutedStyle.Render("   " + strings.Join(parts, "-+-"))
}

func tableCellValue(field github.Field, item github.Item) string {
	if isTitleField(field) {
		if item.Content == nil {
			return "Unavailable"
		}
		if title := strings.TrimSpace(item.Content.Title); title != "" {
			return title
		}
		return contentKind(item.Content)
	}
	value, ok := itemFieldValue(field, item)
	if !ok || !value.Available {
		return "-"
	}
	if value.Value != "" {
		return value.Value
	}
	if value.OptionID != "" {
		for _, option := range field.Options {
			if option.ID == value.OptionID {
				return option.Name
			}
		}
		return value.OptionID
	}
	if value.IterationID != "" {
		for _, iteration := range field.Iterations {
			if iteration.ID == value.IterationID {
				return iteration.Title
			}
		}
		return value.IterationID
	}
	return "-"
}
