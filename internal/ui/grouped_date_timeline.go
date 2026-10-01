package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Explicit section and row order are supplied by the caller. This representation
// does not infer GitHub's saved custom grouping order or collapsed state.
type dateTimelineSection struct {
	ID, Name string
	Rows     []dateTimelineRow
}

type groupedTimelineEntry struct {
	section int
	row     int // -1 is a section heading, never a selectable item
}

func groupedTimelineEntries(sections []dateTimelineSection) ([]groupedTimelineEntry, error) {
	entries := make([]groupedTimelineEntry, 0)
	sectionIDs, itemIDs := map[string]bool{}, map[string]bool{}
	for s, section := range sections {
		if section.ID == "" || sectionIDs[section.ID] {
			return nil, fmt.Errorf("section IDs must be nonempty and unique")
		}
		sectionIDs[section.ID] = true
		entries = append(entries, groupedTimelineEntry{section: s, row: -1})
		for r, row := range section.Rows {
			if row.Item.ID == "" || itemIDs[row.Item.ID] {
				return nil, fmt.Errorf("item IDs must be nonempty and unique across sections")
			}
			itemIDs[row.Item.ID] = true
			entries = append(entries, groupedTimelineEntry{section: s, row: r})
		}
	}
	return entries, nil
}

func groupedTimelineHeading(section dateTimelineSection) string {
	return fmt.Sprintf("  [%s] · %d loaded", timelineText(section.Name), len(section.Rows))
}

// Window entries include headings. If a window starts inside a section, reserve
// a line for its carried heading; selection always retains a body line even at
// tiny heights. The carried heading replaces, rather than adds to, the budget.
func groupedTimelineWindow(entries []groupedTimelineEntry, selected, capacity int) (start, end int, carry bool) {
	if len(entries) == 0 || capacity <= 0 {
		return 0, 0, false
	}
	start = max(0, min(selected-capacity/2, len(entries)-capacity))
	for {
		carry = capacity > 1 && entries[start].row >= 0
		budget := capacity
		if carry {
			budget--
		}
		end = min(len(entries), start+budget)
		if selected < end {
			return
		}
		start += selected - end + 1
	}
}

func renderGroupedDateTimeline(sections []dateTimelineSection, view dateTimelineViewport) string {
	return renderGroupedDateTimelineWithNotice(sections, view, "Synthetic groups · collapsed/custom order unavailable")
}

func renderGroupedDateTimelineWithNotice(sections []dateTimelineSection, view dateTimelineViewport, notice string) string {
	if view.Width <= 0 || view.Height <= 0 {
		return ""
	}
	fit := func(value string) string { return ansi.Truncate(value, view.Width, "…") }
	entries, err := groupedTimelineEntries(sections)
	if err != nil {
		return fit("Grouped timeline unavailable: " + err.Error())
	}
	gutter := min(40, max(24, view.Width/3))
	calendar, err := newTimelineCalendar(view.Month, max(0, view.Width-gutter-5))
	if err != nil {
		return fit("Timeline unavailable: " + err.Error())
	}
	selected := -1
	loaded, outside, undated, unavailable, invalid := 0, 0, 0, 0, 0
	for i, entry := range entries {
		if entry.row < 0 {
			continue
		}
		row := sections[entry.section].Rows[entry.row]
		if selected < 0 || row.Item.ID == view.SelectedID {
			selected = i
		}
		loaded++
		switch timelineSpan(row, calendar).status {
		case "Before range", "After range":
			outside++
		case "Undated":
			undated++
		case "Unavailable":
			unavailable++
		case "Invalid dates":
			invalid++
		}
	}
	if view.Width < 60 {
		if strings.HasPrefix(notice, "Synthetic") {
			notice = "Synthetic grouped date list (narrow terminal)"
		} else {
			notice += " · date list"
		}
	}
	counts := fmt.Sprintf("Loaded %d · groups %d · outside %d · undated %d · unavailable %d · invalid %d", loaded, len(sections), outside, undated, unavailable, invalid)
	if view.Loading {
		counts += " · loading"
	}
	headers := []string{notice, counts}
	if view.Width >= 60 {
		headers = append(headers, "  "+padANSI("Item", gutter)+" | "+calendar.header())
	}
	footer := []string{}
	if selected >= 0 {
		entry := entries[selected]
		row := sections[entry.section].Rows[entry.row]
		footer = []string{"Start: " + timelineEndpointText(row.Start), "Target: " + timelineEndpointText(row.Target)}
		footer = footer[:min(2, view.Height-1)]
	}
	if len(entries) == 0 {
		headers = append(headers, "No loaded groups")
	}
	bodyMinimum := 0
	if len(entries) > 0 {
		bodyMinimum = 1
		if selected >= 0 && view.Height-len(footer) >= 2 {
			bodyMinimum = 2
		}
	}
	headers = headers[:min(len(headers), max(0, view.Height-len(footer)-bodyMinimum))]
	lines := make([]string, 0, view.Height)
	for _, header := range headers {
		lines = append(lines, fit(header))
	}
	capacity := view.Height - len(headers) - len(footer)
	focus := max(0, selected)
	start, end, carry := groupedTimelineWindow(entries, focus, capacity)
	if carry {
		lines = append(lines, fit(groupedTimelineHeading(sections[entries[start].section])))
	}
	for i := start; i < end; i++ {
		entry := entries[i]
		section := sections[entry.section]
		if entry.row < 0 {
			lines = append(lines, fit(groupedTimelineHeading(section)))
			continue
		}
		lines = append(lines, fit(timelineRowLine(section.Rows[entry.row], i == selected, calendar, view.Width, gutter)))
	}
	for _, line := range footer {
		lines = append(lines, fit(line))
	}
	return strings.Join(lines, "\n")
}
