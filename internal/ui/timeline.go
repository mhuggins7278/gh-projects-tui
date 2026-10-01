package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func (m Model) isTimeline() bool  { return m.view != nil && m.view.Layout == github.RoadmapLayout }
func (m Model) isRowLayout() bool { return m.isTable() || m.isTimeline() }

func (m *Model) handleTimelineKey(key string) (tea.Cmd, bool) {
	switch key {
	case "j", "down":
		m.moveTableRow(1)
	case "k", "up":
		m.moveTableRow(-1)
	case "enter":
		return m.openItemDetailCmd(), true
	case "h", "left", "l", "right":
		month, err := time.Parse("2006-01", m.timelineMonth)
		if err != nil {
			month = time.Now()
		}
		delta := 1
		if key == "h" || key == "left" {
			delta = -1
		}
		next := month.AddDate(0, delta, 0).Format("2006-01")
		if _, err := newTimelineCalendar(next, 80); err == nil {
			m.timelineMonth = next
		}
	case "g":
		m.timelineMonth = time.Now().Format("2006-01")
	case "H", "L", "J", "K", "m", "a", "D", "c", "x":
		m.status = "Timelines are read-only"
	default:
		return nil, false
	}
	return nil, true
}

// Complete item field connections are read for timelines; absent mapped IDs
// therefore mean unset. Unavailable values remain distinct from absent values.
func timelineEndpoint(fieldID string, item github.Item) dateTimelineEndpoint {
	endpoint := dateTimelineEndpoint{Available: true}
	found := false
	for _, value := range item.FieldValues {
		if value.FieldID != fieldID {
			continue
		}
		if found {
			return dateTimelineEndpoint{}
		}
		found = true
		endpoint = dateTimelineEndpoint{Value: value.Value, Available: value.Available}
	}
	return endpoint
}

func (m Model) renderTimelineContent(b *strings.Builder) {
	mapping, ok := m.roadmapMappings.Find(m.host, m.view.ProjectID, m.view.ID)
	if !ok {
		b.WriteString("Timeline mapping unavailable\n")
		return
	}
	fields, err := github.ResolveRoadmapDateFields(m.view.ProjectFields, mapping.StartFieldID, mapping.TargetFieldID)
	if err != nil {
		b.WriteString("Timeline mapping unavailable: " + err.Error() + "\n")
		return
	}
	width := m.frameContentWidth()
	items := m.tableItems()
	lines := []string{
		m.view.Name + " · read-only timeline",
		fmt.Sprintf("User-configured: start %s · target %s", fields.Start.Name, fields.Target.Name),
		"Local range; saved zoom, markers, slicing and field sums unavailable.",
	}
	if strings.TrimSpace(m.view.Filter) != "" {
		lines = append(lines, "Saved filter: "+m.view.Filter)
	}
	if len(m.view.SortByFields) == 1 {
		lines = append(lines, "Saved order: start date ascending; unset last; project-position ties")
	}
	if m.filtering || strings.TrimSpace(m.filter) != "" {
		lines = append(lines, fmt.Sprintf("Search loaded rows: %s (%d matches of %d loaded)", m.filter, len(items), len(m.items)))
	}
	if m.itemsErr != nil {
		lines = append(lines, "Items incomplete: "+m.itemsErr.Error()+"; r retries")
	}
	for _, line := range lines {
		b.WriteString(truncateText(timelineText(line), width) + "\n")
	}
	rows := make([]dateTimelineRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, dateTimelineRow{Item: item, Start: timelineEndpoint(mapping.StartFieldID, item), Target: timelineEndpoint(mapping.TargetFieldID, item)})
	}
	month := m.timelineMonth
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	selected := ""
	if index := m.selectedTableRow(items); index >= 0 {
		selected = items[index].ID
	}
	height := m.bodyCanvasHeight() - len(lines)
	if m.status != "" {
		height -= 2
	}
	if m.showHelp {
		height -= lipgloss.Height(helpStyle.Width(width-2).Render(m.helpText())) + 1
	}
	if m.showAPI && m.debug {
		height -= lipgloss.Height(helpStyle.Width(width-2).Render(m.apiInspector())) + 1
	}
	b.WriteString(renderDateTimeline(rows, dateTimelineViewport{Month: month, Width: width, Height: max(1, height), SelectedID: selected, Loading: m.itemsLoading}))
	b.WriteString("\n")
}
