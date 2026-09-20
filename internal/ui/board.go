package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type boardLane struct {
	Name  string
	Items []github.Item
}

type laneDefinition struct {
	key  string
	name string
}

func (m Model) boardLanes() []boardLane {
	return lanesForView(m.view, m.items)
}

func (m Model) selectedBoardItem() (github.Item, bool) {
	lanes := m.boardLanes()
	if m.boardLane < 0 || m.boardLane >= len(lanes) {
		return github.Item{}, false
	}
	items := lanes[m.boardLane].Items
	if m.boardCard < 0 || m.boardCard >= len(items) {
		return github.Item{}, false
	}
	return items[m.boardCard], true
}

func boardGroupField(view *github.View) (github.Field, string, bool) {
	if view == nil {
		return github.Field{}, "", false
	}
	if len(view.GroupByFields) > 0 {
		return view.GroupByFields[0], "column", true
	}
	if len(view.VerticalGroupBy) > 0 {
		return view.VerticalGroupBy[0], "vertical", true
	}
	return github.Field{}, "", false
}

func lanesForView(view *github.View, items []github.Item) []boardLane {
	if view == nil {
		return nil
	}
	field, _, hasGroup := boardGroupField(view)
	if !hasGroup {
		return []boardLane{{Name: "All items", Items: append([]github.Item(nil), items...)}}
	}

	definitions, supported := laneDefinitions(field)
	lanes := make([]boardLane, 0, len(definitions)+1)
	indexes := make(map[string]int, len(definitions))
	for _, definition := range definitions {
		indexes[definition.key] = len(lanes)
		lanes = append(lanes, boardLane{Name: definition.name})
	}
	if !supported {
		return []boardLane{{Name: "All items", Items: append([]github.Item(nil), items...)}}
	}

	for _, item := range items {
		key, label := itemGroupKey(field, item)
		index, ok := indexes[key]
		if !ok {
			// Keep an item visible when GitHub returns a value not present in
			// the saved view metadata.
			if key == "no-value" {
				index = indexes[key]
			} else {
				index = len(lanes) - 1
				if index < 0 || lanes[index].Name == "No value" {
					index = len(lanes)
					lanes = append(lanes, boardLane{Name: "Other"})
				}
			}
			// The card remains in one stable fallback lane; its actual
			// value is still shown in the card fields below.
			_ = label
		}
		lanes[index].Items = append(lanes[index].Items, item)
	}
	return lanes
}

func laneDefinitions(field github.Field) ([]laneDefinition, bool) {
	switch {
	case field.Kind == "ProjectV2SingleSelectField" || field.DataType == "SINGLE_SELECT":
		definitions := make([]laneDefinition, 0, len(field.Options)+1)
		for _, option := range field.Options {
			definitions = append(definitions, laneDefinition{key: "option:" + option.ID, name: option.Name})
		}
		definitions = append(definitions, laneDefinition{key: "no-value", name: "No value"})
		return definitions, true
	case field.Kind == "ProjectV2IterationField" || field.DataType == "ITERATION":
		definitions := make([]laneDefinition, 0, len(field.Iterations)+1)
		for _, iteration := range field.Iterations {
			definitions = append(definitions, laneDefinition{key: "iteration:" + iteration.ID, name: iteration.Title})
		}
		definitions = append(definitions, laneDefinition{key: "no-value", name: "No value"})
		return definitions, true
	default:
		return nil, false
	}
}

func itemGroupKey(field github.Field, item github.Item) (string, string) {
	value, ok := itemFieldValue(field, item)
	if !ok || !value.Available {
		return "no-value", ""
	}
	if field.Kind == "ProjectV2IterationField" || field.DataType == "ITERATION" {
		if value.IterationID == "" {
			return "no-value", value.Value
		}
		return "iteration:" + value.IterationID, value.Value
	}
	if value.OptionID == "" {
		if value.Value == "" {
			return "no-value", ""
		}
		return "value:" + value.Value, value.Value
	}
	return "option:" + value.OptionID, value.Value
}

func itemFieldValue(field github.Field, item github.Item) (github.FieldValue, bool) {
	for _, value := range item.FieldValues {
		if value.FieldID == field.ID || (field.ID == "" && value.FieldName == field.Name) {
			return value, true
		}
	}
	return github.FieldValue{}, false
}

func (m *Model) moveBoardLane(delta int) {
	lanes := m.boardLanes()
	if len(lanes) == 0 {
		return
	}
	m.boardLane += delta
	if m.boardLane < 0 {
		m.boardLane = 0
	}
	if m.boardLane >= len(lanes) {
		m.boardLane = len(lanes) - 1
	}
	m.boardCard = 0
	if len(lanes[m.boardLane].Items) > 0 {
		m.boardCard = minInt(m.boardCard, len(lanes[m.boardLane].Items)-1)
	}
}

func (m *Model) moveBoardCard(delta int) {
	lanes := m.boardLanes()
	if m.boardLane < 0 || m.boardLane >= len(lanes) || len(lanes[m.boardLane].Items) == 0 {
		return
	}
	m.boardCard += delta
	if m.boardCard < 0 {
		m.boardCard = 0
	}
	if m.boardCard >= len(lanes[m.boardLane].Items) {
		m.boardCard = len(lanes[m.boardLane].Items) - 1
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func (m Model) renderBoard(b *strings.Builder) {
	board := &strings.Builder{}
	m.renderBoardContent(board)
	if !m.detailVisible {
		b.WriteString(board.String())
		return
	}
	b.WriteString(renderPopover(board.String(), m.renderDetailPanel(), m.frameContentWidth(), m.bodyCanvasHeight()))
}

func (m Model) renderBoardContent(b *strings.Builder) {
	if m.loadingDetail {
		b.WriteString("Loading view...\n")
		return
	}
	if m.view == nil {
		b.WriteString("No view loaded. Press v to pick a view.\n")
		return
	}

	fmt.Fprintf(b, "%s  %s  %s\n", titleStyle.Render(m.view.Name), mutedStyle.Render(fmt.Sprintf("#%d", m.view.Number)), layoutBadge(string(m.view.Layout)))
	if m.view.Filter != "" {
		fmt.Fprintf(b, "%s %s\n", mutedStyle.Render("Filter:"), m.view.Filter)
	}
	field, axis, hasGroup := boardGroupField(m.view)
	if !hasGroup {
		b.WriteString(mutedStyle.Render("Grouping: none") + "\n")
	} else {
		label := field.Name
		if label == "" {
			label = "unknown field"
		}
		if _, supported := laneDefinitions(field); supported {
			fmt.Fprintf(b, "%s %s (%s)\n", mutedStyle.Render("Grouping:"), label, axis)
		} else {
			fmt.Fprintf(b, "%s %s (%s; display fallback; field type is not supported yet)\n", statusStyle.Render("Grouping:"), label, axis)
		}
	}
	if len(m.view.SortByFields) > 0 && !positionOnly(m.view) {
		b.WriteString(statusStyle.Render("Sort: saved field sort is not applied yet; showing project position") + "\n")
	}
	fmt.Fprintf(b, "%s %d", mutedStyle.Render("Items:"), len(m.items))
	if m.itemsLoading {
		b.WriteString("  " + statusStyle.Render("[loading more]"))
	}
	b.WriteString("\n")

	if m.itemsErr != nil && len(m.items) == 0 {
		b.WriteString("\n" + errorStyle.Render("Items failed: "+m.itemsErr.Error()) + "\n")
		b.WriteString(mutedStyle.Render("Press r to retry.") + "\n")
		return
	}
	if len(m.items) == 0 {
		if m.itemsLoading {
			b.WriteString("\n" + sectionStyle.Render("Loading issues and pull requests...") + "\n")
		} else {
			b.WriteString("\n" + mutedStyle.Render("No issues, pull requests, or drafts match this view.") + "\n")
		}
		return
	}

	lanes := m.boardLanes()
	b.WriteString("\n" + m.renderLaneGrid(lanes) + "\n")
	if m.itemsErr != nil {
		b.WriteString(errorStyle.Render("Loading stopped: "+m.itemsErr.Error()) + "\n")
	}
}

func (m Model) renderLaneGrid(lanes []boardLane) string {
	columns, laneWidth := m.boardGrid(len(lanes))
	viewportHeight := m.boardViewportHeight()
	rowStart := 0
	rowEnd := len(lanes)
	if columns < len(lanes) {
		// Keep the active lane's row on screen on narrow terminals. h/l then
		// acts as a horizontal lane navigator without losing focus below the
		// visible viewport.
		rowStart = (m.boardLane / columns) * columns
		rowEnd = rowStart + columns
		if rowEnd > len(lanes) {
			rowEnd = len(lanes)
		}
	}
	blocks := make([]string, 0, (rowEnd-rowStart)*2-1)
	for laneIndex := rowStart; laneIndex < rowEnd; laneIndex++ {
		blocks = append(blocks, m.renderLaneColumn(lanes[laneIndex], laneIndex, laneWidth, viewportHeight))
		if laneIndex+1 < rowEnd {
			blocks = append(blocks, "  ")
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}

func (m Model) boardGrid(laneCount int) (int, int) {
	available := m.frameContentWidth() - 2
	if available < 20 {
		available = 20
	}
	columns := available / 26
	if columns < 1 {
		columns = 1
	}
	if columns > laneCount {
		columns = laneCount
	}
	if columns < 1 {
		columns = 1
	}
	totalWidth := (available - (columns-1)*2) / columns
	contentWidth := totalWidth - 4
	if contentWidth < 12 {
		contentWidth = 12
	}
	return columns, contentWidth
}

func (m Model) renderLaneColumn(lane boardLane, laneIndex, width, viewportHeight int) string {
	active := laneIndex == m.boardLane
	marker := "  "
	if active {
		marker = "> "
	}
	header := laneTitleStyle(laneIndex).Render(fmt.Sprintf("%s[%s]  %s", marker, lane.Name, itemCountLabel(len(lane.Items))))
	lines := []string{header}
	if len(lane.Items) == 0 {
		lines = append(lines, mutedStyle.Render("No cards"))
		return laneFrameStyle(laneIndex).Width(width).Render(strings.Join(lines, "\n"))
	}

	start, end := m.boardWindowForLane(lane, m.boardCard, width, viewportHeight, active)
	showEarlier := start > 0
	showLater := end < len(lane.Items)
	column := m.renderLaneWindow(lane, laneIndex, width, start, end, showEarlier, showLater, active)
	for lipgloss.Height(column) > viewportHeight {
		if showLater {
			showLater = false
		} else if showEarlier {
			showEarlier = false
		} else if end-start > 1 {
			if active && m.boardCard == end-1 {
				start++
			} else {
				end--
			}
		} else {
			break
		}
		column = m.renderLaneWindow(lane, laneIndex, width, start, end, showEarlier, showLater, active)
	}
	return column
}

func (m Model) renderLaneWindow(lane boardLane, laneIndex, width, start, end int, showEarlier, showLater, active bool) string {
	lines := []string{laneTitleStyle(laneIndex).Render(fmt.Sprintf("%s[%s]  %s", map[bool]string{true: "> ", false: "  "}[active], lane.Name, itemCountLabel(len(lane.Items))))}
	if showEarlier {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("... %d earlier", start)))
	}
	for itemIndex := start; itemIndex < end; itemIndex++ {
		lines = append(lines, m.renderLaneCard(lane.Items[itemIndex], itemIndex == m.boardCard && active, width))
	}
	if showLater {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("... %d more", len(lane.Items)-end)))
	}
	return laneFrameStyle(laneIndex).Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) renderLaneCard(item github.Item, selected bool, width int) string {
	cardWidth := width - 4
	if cardWidth < 8 {
		cardWidth = 8
	}
	if item.Content == nil {
		return cardStyle(selected).Width(cardWidth).Render(mutedStyle.Render("content unavailable"))
	}
	kind := contentKind(item.Content)
	identity := kind
	if item.Content.Number > 0 {
		identity += fmt.Sprintf(" #%d", item.Content.Number)
	}
	title := truncateText(item.Content.Title, cardWidth-2)
	if title == "" {
		title = "(untitled)"
	}
	if selected {
		identity = "> " + identity
	}
	lines := []string{itemKindStyle(kind).Render(identity), titleStyle.Render(title)}
	if summary := cardFieldSummary(item, m.view); summary != "" {
		lines = append(lines, mutedStyle.Render(truncateText(summary, cardWidth-2)))
	}
	return cardStyle(selected).Width(cardWidth).Render(strings.Join(lines, "\n"))
}

func positionOnly(view *github.View) bool {
	if view == nil || len(view.SortByFields) == 0 {
		return true
	}
	for _, sort := range view.SortByFields {
		if sort.Field.DataType != "POSITION" && sort.Field.Name != "Position" {
			return false
		}
	}
	return true
}

func (m Model) boardViewportHeight() int {
	if m.height <= 0 {
		return 32
	}
	reserved := 16
	if m.view != nil {
		if m.view.Filter != "" {
			reserved++
		}
		if len(m.view.SortByFields) > 0 && !positionOnly(m.view) {
			reserved++
		}
	}
	if m.showHelp {
		reserved += 5
	}
	if m.status != "" {
		reserved++
	}
	viewport := m.height - reserved
	if viewport < 8 {
		return 8
	}
	return viewport
}

func (m Model) boardWindowForLane(lane boardLane, selected, width, viewportHeight int, active bool) (int, int) {
	if len(lane.Items) == 0 {
		return 0, 0
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(lane.Items) {
		selected = len(lane.Items) - 1
	}

	heights := make([]int, len(lane.Items))
	for i, item := range lane.Items {
		heights[i] = lipgloss.Height(m.renderLaneCard(item, false, width))
		if heights[i] < 1 {
			heights[i] = 1
		}
	}

	// A lane frame consumes two border rows and one header row. Reserve
	// another row for each overflow marker so the selected card remains in the
	// actual terminal viewport, not just in the logical item window.
	capacity := viewportHeight - 3
	if active {
		if selected > 0 {
			capacity--
		}
		if selected < len(lane.Items)-1 {
			capacity--
		}
	} else if len(lane.Items) > 1 {
		capacity--
	}
	if capacity < 1 {
		capacity = 1
	}

	if !active {
		used := 0
		end := 0
		for end < len(heights) && (end == 0 || used+heights[end] <= capacity) {
			used += heights[end]
			end++
		}
		if end == 0 {
			end = 1
		}
		return 0, end
	}

	start, end := selected, selected+1
	used := heights[selected]
	for start > 0 || end < len(heights) {
		beforeHeight := 0
		if start > 0 {
			beforeHeight = heights[start-1]
		}
		afterHeight := 0
		if end < len(heights) {
			afterHeight = heights[end]
		}
		preferBefore := start > 0 && (end >= len(heights) || selected-start <= end-selected)
		if preferBefore && used+beforeHeight <= capacity {
			start--
			used += beforeHeight
			continue
		}
		if end < len(heights) && used+afterHeight <= capacity {
			end++
			used += afterHeight
			continue
		}
		if start > 0 && used+beforeHeight <= capacity {
			start--
			used += beforeHeight
			continue
		}
		break
	}
	return start, end
}

func boardCardLine(item github.Item, view *github.View) string {
	if item.Content == nil {
		return "(content unavailable)"
	}
	title := strings.TrimSpace(item.Content.Title)
	if title == "" {
		title = "(untitled)"
	}
	kind := contentKind(item.Content)
	identity := kind
	if item.Content.Number > 0 {
		identity += fmt.Sprintf(" #%d", item.Content.Number)
	}
	values := cardFieldSummary(item, view)
	if values == "" {
		return fmt.Sprintf("%s - %s", identity, title)
	}
	return fmt.Sprintf("%s - %s [%s]", identity, title, values)
}

func contentKind(content *github.Content) string {
	switch content.Kind {
	case "Issue":
		return "Issue"
	case "PullRequest":
		return "PR"
	case "DraftIssue":
		return "Draft"
	default:
		if content.Kind == "" {
			return "Item"
		}
		return content.Kind
	}
}

func cardFieldSummary(item github.Item, view *github.View) string {
	if view == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, field := range view.Fields {
		value, ok := itemFieldValue(field, item)
		if !ok {
			continue
		}
		text := value.Value
		if !value.Available {
			text = "unavailable"
		}
		if text == "" {
			text = "No value"
		}
		name := field.Name
		if name == "" {
			name = value.FieldName
		}
		parts = append(parts, name+": "+text)
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, ", ")
}
