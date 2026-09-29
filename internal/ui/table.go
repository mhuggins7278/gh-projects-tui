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
	if len(m.view.GroupByFields) > 0 {
		b.WriteString(mutedStyle.Render("Display note: groups follow saved field/option order; collapsed state is unavailable from the API.") + "\n")
	} else {
		b.WriteString(mutedStyle.Render("Display note: rendering saved visible fields in API order.") + "\n")
	}
	if m.view.Filter != "" {
		fmt.Fprintf(b, "%s %s\n", mutedStyle.Render("Filter:"), m.view.Filter)
	}
	visibleItems := m.boardItems()
	groups := tableGroups(m.view, visibleItems)
	visibleItems = tableGroupItems(groups)
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
	if m.tableMove != nil {
		label := m.tableMove.itemID
		if item, ok := m.selectedItem(); ok && item.Content != nil {
			label = item.Content.Title
		}
		b.WriteString(sectionStyle.Render("Move "+truncateText(label, m.frameContentWidth()-10)+" to group:") + "\n")
		start, end := m.tableWindow(len(m.tableMove.lanes), m.tableMove.index)
		if start > 0 {
			fmt.Fprintf(b, "... %d earlier\n", start)
		}
		for index := start; index < end; index++ {
			lane := m.tableMove.lanes[index]
			marker := "  "
			if index == m.tableMove.index {
				marker = "> "
			}
			b.WriteString(statusStyle.Render(truncateText(marker+lane.Name, m.frameContentWidth()-2)) + "\n")
		}
		if end < len(m.tableMove.lanes) {
			fmt.Fprintf(b, "... %d more\n", len(m.tableMove.lanes)-end)
		}
		return
	}

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

	selected := m.selectedTableRow(visibleItems)
	entries := tableEntries(groups)
	selectedEntry := 0
	for index, entry := range entries {
		if entry.row == selected {
			selectedEntry = index
			break
		}
	}
	start, end := m.tableWindow(len(entries), selectedEntry)
	if start > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("... %d earlier", start)) + "\n")
		if entries[start].row >= 0 && groups[entries[start].group].Name != "" {
			group := groups[entries[start].group]
			b.WriteString(tableGroupHeading(group, m.frameContentWidth()-2, entries[start].group) + "\n")
		}
	}
	for index := start; index < end; index++ {
		entry := entries[index]
		if entry.row < 0 {
			b.WriteString(tableGroupHeading(groups[entry.group], m.frameContentWidth()-2, entry.group) + "\n")
			continue
		}
		b.WriteString(tableDataRow(columns, visibleItems[entry.row], widths, entry.row == selected) + "\n")
	}
	if end < len(entries) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("... %d more", len(entries)-end)) + "\n")
	}
	if m.itemsErr != nil {
		b.WriteString(errorStyle.Render("Items incomplete: "+m.itemsErr.Error()) + "\n")
	}
}

type tableEntry struct {
	group int
	row   int // -1 for a group heading
}

func tableGroups(view *github.View, items []github.Item) []boardLane {
	if view == nil || len(view.GroupByFields) == 0 {
		return []boardLane{{Items: items}}
	}
	groups := lanesForView(view, items)
	visible := groups[:0]
	for _, group := range groups {
		if len(group.Items) > 0 {
			visible = append(visible, group)
		}
	}
	return visible
}

func tableGroupItems(groups []boardLane) []github.Item {
	count := 0
	for _, group := range groups {
		count += len(group.Items)
	}
	items := make([]github.Item, 0, count)
	for _, group := range groups {
		items = append(items, group.Items...)
	}
	return items
}

func tableEntries(groups []boardLane) []tableEntry {
	entries := make([]tableEntry, 0)
	row := 0
	for groupIndex, group := range groups {
		if group.Name != "" {
			entries = append(entries, tableEntry{group: groupIndex, row: -1})
		}
		for range group.Items {
			entries = append(entries, tableEntry{group: groupIndex, row: row})
			row++
		}
	}
	return entries
}

func tableGroupHeading(group boardLane, width, index int) string {
	color := tableValueColor(group.Name, index)
	if len(group.Items) == 0 {
		color = muted
	}
	label := truncateText("  ● ["+group.Name+"]", width-8)
	count := fmt.Sprintf("  %d", len(group.Items))
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(label) + mutedStyle.Render(count)
}

func (m Model) tableWindow(count, selected int) (int, int) {
	// Group headings, an optional carried heading, and overflow markers also
	// occupy terminal lines; leave room for them around the selected row.
	rows := m.boardViewportHeight() - 6
	if rows < 1 {
		rows = 1
	}
	if rows > count {
		rows = count
	}
	start := 0
	if selected >= rows {
		start = selected - rows + 1
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

func (m Model) isTable() bool {
	return m.view != nil && m.view.Layout == github.TableLayout
}

func (m Model) selectedTableRow(items []github.Item) int {
	if len(items) == 0 {
		return -1
	}
	if m.tableFocusID != "" {
		for index, item := range items {
			if item.ID == m.tableFocusID {
				return index
			}
		}
	}
	if m.tableRow < 0 {
		return 0
	}
	if m.tableRow >= len(items) {
		return len(items) - 1
	}
	return m.tableRow
}

func (m Model) selectedItem() (github.Item, bool) {
	if !m.isTable() {
		return m.selectedBoardItem()
	}
	items := m.tableItems()
	index := m.selectedTableRow(items)
	if index < 0 {
		return github.Item{}, false
	}
	return items[index], true
}

func (m *Model) moveTableRow(delta int) {
	items := m.tableItems()
	index := m.selectedTableRow(items)
	if index < 0 {
		return
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(items) {
		index = len(items) - 1
	}
	m.tableRow = index
	m.tableFocusID = items[index].ID
}

// Keep the selected identity through sorting, searches, and in-flight pages.
// Once loading finishes, fall back to the nearest row if that identity vanished.
func (m *Model) clampTableRow(finished bool) {
	items := m.tableItems()
	index := m.selectedTableRow(items)
	if index < 0 {
		m.tableRow = 0
		return
	}
	m.tableRow = index
	for _, item := range items {
		if item.ID == m.tableFocusID && m.tableFocusID != "" {
			return
		}
	}
	if finished && strings.TrimSpace(m.filter) == "" {
		m.tableFocusID = items[index].ID
	}
}

func (m Model) tableItems() []github.Item {
	return tableGroupItems(tableGroups(m.view, m.boardItems()))
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
	if width < 12 {
		width = 12
	}
	if strings.EqualFold(field.Name, "Assignees") && width < 16 {
		width = 16
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
	// Two row-style padding cells, two marker cells, and separators.
	budget := available - 4 - (len(columns)-1)*3
	if budget < len(columns) {
		budget = len(columns)
	}
	widths := make([]int, len(columns))
	titleIndex := 0
	otherTotal := 0
	for index, column := range columns {
		if isTitleField(column) {
			titleIndex = index
			continue
		}
		widths[index] = tableColumnWidth(column, "")
		if widths[index] > 24 {
			widths[index] = 24
		}
		otherTotal += widths[index]
	}
	// Give the title a wider baseline while keeping each other column readable
	// when the terminal has room. Fall back to the compact layout on very narrow
	// terminals, where all saved columns cannot retain that minimum.
	otherCount := len(columns) - 1
	minTitle, minOther := 32, 8
	if budget < 20+otherCount*minOther {
		minTitle, minOther = 20, 1
		if budget < minTitle+otherCount {
			minTitle = budget - otherCount
		}
	} else if budget < minTitle+otherCount*minOther {
		minTitle = budget - otherCount*minOther
	}
	for otherTotal > budget-minTitle {
		changed := false
		for index := range widths {
			if index != titleIndex && widths[index] > minOther && otherTotal > budget-minTitle {
				widths[index]--
				otherTotal--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	widths[titleIndex] = budget - otherTotal
	if widths[titleIndex] > 96 {
		extra := widths[titleIndex] - 96
		widths[titleIndex] = 96
		for extra > 0 {
			changed := false
			for index := range widths {
				if index != titleIndex && widths[index] < 32 && extra > 0 {
					widths[index]++
					extra--
					changed = true
				}
			}
			if !changed {
				widths[titleIndex] += extra
				break
			}
		}
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

func tableDataRow(columns []github.Field, item github.Item, widths []int, selected bool) string {
	parts := make([]string, 0, len(columns))
	for index, column := range columns {
		parts = append(parts, tableStyledCell(column, item, widths[index]))
	}
	prefix := "  "
	if selected {
		prefix = "> "
	}
	row := prefix + strings.Join(parts, mutedStyle.Render(" | "))
	if selected {
		return selectedRowStyle.Render(row)
	}
	return rowStyle.Render(row)
}

func tableStyledCell(field github.Field, item github.Item, width int) string {
	if width < 1 {
		return ""
	}
	value := tableCellValue(field, item)
	var result string
	switch {
	case isTitleField(field) && item.Content != nil:
		icon := contentIcon(item.Content)
		iconStyle := itemKindStyle(contentKind(item.Content))
		if state := contentState(item.Content); state != "" {
			iconStyle = itemStateStyle(state)
		}
		if width <= lipgloss.Width(icon)+1 {
			result = iconStyle.Render(truncateText(icon, width))
			break
		}
		identity := ""
		if item.Content.Number > 0 {
			identity = fmt.Sprintf(" #%d", item.Content.Number)
		}
		// Keep the identifier visible even when the title needs truncation.
		if lipgloss.Width(identity) > width-2 {
			identity = ""
		}
		titleWidth := width - lipgloss.Width(icon) - 1 - lipgloss.Width(identity)
		if titleWidth < 0 {
			titleWidth = 0
		}
		result = iconStyle.Render(icon) + " " + titleStyle.Render(truncateText(value, titleWidth)) + mutedStyle.Render(identity)
	case value == "-" || value == "Unavailable":
		result = mutedStyle.Render(truncateText(value, width))
	case isSingleSelectField(field):
		result = lipgloss.NewStyle().Bold(true).Foreground(tableValueColor(value, 0)).Render(truncateText("● "+value, width))
	case strings.EqualFold(field.Name, "Sub-issues progress") && item.Content != nil && item.Content.SubIssueTotal > 0:
		result = lipgloss.NewStyle().Foreground(purple).Render(truncateText(value, width))
	default:
		result = truncateText(value, width)
	}
	padding := width - lipgloss.Width(result)
	if padding > 0 {
		result += strings.Repeat(" ", padding)
	}
	return result
}

func tableValueColor(value string, fallback int) lipgloss.Color {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "todo", "to do", "ready", "open":
		return green
	case "in progress", "doing", "active":
		return blue
	case "done", "completed", "closed":
		return purple
	case "blocked", "high", "urgent":
		return red
	case "no status", "no value":
		return muted
	default:
		return laneColor(fallback)
	}
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
	if strings.EqualFold(field.Name, "Sub-issues progress") {
		if item.Content == nil {
			return "Unavailable"
		}
		if item.Content.Kind == "Issue" && item.Content.SubIssueTotal > 0 {
			percent := (item.Content.SubIssueDone*100 + item.Content.SubIssueTotal/2) / item.Content.SubIssueTotal
			return fmt.Sprintf("%d/%d %d%%", item.Content.SubIssueDone, item.Content.SubIssueTotal, percent)
		}
		return "-"
	}
	value, ok := itemFieldValue(field, item)
	if !ok {
		return "-"
	}
	if !value.Available {
		return "Unavailable"
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
