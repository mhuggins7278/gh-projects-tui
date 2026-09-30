package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

// These inputs must come from explicit endpoint mappings. The renderer does not
// inspect project field names/configuration or enable saved roadmap selection.
// Inclusive bars and one-endpoint points are provisional display conventions;
// #25/#26 must verify endpoint identity and placement before live integration.
type dateTimelineEndpoint struct {
	Value     string
	Available bool // available + empty means unset; unavailable is a separate state
}

type dateTimelineRow struct {
	Item          github.Item
	Start, Target dateTimelineEndpoint
}

type dateTimelineViewport struct {
	Month         string // YYYY-MM, a local three-calendar-month window
	Width, Height int
	SelectedID    string
	Loading       bool
}

type timelineDateSpan struct {
	first, last time.Time
	status      string
}

type timelineCalendar struct {
	start, end time.Time // UTC calendar dates; end is exclusive
	months     [3]time.Time
	widths     [3]int
	axisWidth  int
}

func newTimelineCalendar(month string, width int) (timelineCalendar, error) {
	start, err := time.Parse("2006-01", month)
	if err != nil || start.Year() < 1 || start.Year() > 9999 || start.AddDate(0, 3, -1).Year() > 9999 {
		return timelineCalendar{}, fmt.Errorf("invalid viewport month %q", timelineText(month))
	}
	c := timelineCalendar{start: start, end: start.AddDate(0, 3, 0), axisWidth: width}
	for i := range c.months {
		c.months[i] = start.AddDate(0, i, 0)
		c.widths[i] = max(0, (width-2)/3)
		if i < max(0, (width-2)%3) {
			c.widths[i]++
		}
	}
	return c, nil
}

func timelineSpan(row dateTimelineRow, c timelineCalendar) timelineDateSpan {
	span := timelineDateSpan{}
	if row.Item.Content == nil || !row.Start.Available || !row.Target.Available {
		span.status = "Unavailable"
		return span
	}
	if row.Start.Value == "" && row.Target.Value == "" {
		span.status = "Undated"
		return span
	}
	parse := func(value string) (time.Time, error) {
		if value == "" {
			return time.Time{}, nil
		}
		date, err := time.Parse("2006-01-02", value)
		if err == nil && date.Year() < 1 {
			err = fmt.Errorf("invalid calendar year")
		}
		return date, err
	}
	first, firstErr := parse(row.Start.Value)
	last, lastErr := parse(row.Target.Value)
	if firstErr != nil || lastErr != nil || (row.Start.Value != "" && row.Target.Value != "" && last.Before(first)) {
		span.status = "Invalid dates"
		return span
	}
	if row.Start.Value == "" {
		first = last
	}
	if row.Target.Value == "" {
		last = first
	}
	span.first, span.last = first, last
	switch {
	case last.Before(c.start):
		span.status = "Before range"
	case !first.Before(c.end):
		span.status = "After range"
	default:
		span.status = "Dated"
	}
	return span
}

// cell maps a visible calendar date into a daily bucket within its month.
// No Unix timestamp or local timezone conversion is used.
func (c timelineCalendar) cell(date time.Time) int {
	if date.Before(c.start) {
		return 0
	}
	if !date.Before(c.end) {
		return c.axisWidth - 1
	}
	month := 12*(date.Year()-c.start.Year()) + int(date.Month()-c.start.Month())
	offset := 0
	for i := 0; i < month; i++ {
		offset += c.widths[i] + 1
	}
	days := c.months[month].AddDate(0, 1, -1).Day()
	return offset + (date.Day()-1)*c.widths[month]/days
}

func (c timelineCalendar) header() string {
	parts := make([]string, 3)
	for i := range parts {
		parts[i] = padANSI(ansi.Truncate(c.months[i].Format("Jan 2006"), c.widths[i], "…"), c.widths[i])
	}
	return strings.Join(parts, "|")
}

func (c timelineCalendar) bar(span timelineDateSpan) string {
	if span.status != "Dated" {
		return span.status
	}
	cells := []rune(strings.Repeat("·", c.axisWidth))
	separators := []int{c.widths[0], c.widths[0] + 1 + c.widths[1]}
	for _, index := range separators {
		cells[index] = '|'
	}
	first, last := c.cell(span.first), c.cell(span.last)
	for i := first; i <= last; i++ {
		if cells[i] != '|' {
			cells[i] = '━'
		}
	}
	cells[first], cells[last] = '●', '●'
	if span.first.Before(c.start) {
		cells[0] = '<'
	}
	if !span.last.Before(c.end) {
		cells[len(cells)-1] = '>'
	}
	return string(cells)
}

func timelineText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(value))
}

func timelineEndpointText(endpoint dateTimelineEndpoint) string {
	if !endpoint.Available {
		return "unavailable"
	}
	if endpoint.Value == "" {
		return "unset"
	}
	return timelineText(endpoint.Value)
}

func timelineRowTitle(item github.Item) string {
	if item.Content == nil {
		return "Unavailable item"
	}
	title := timelineText(item.Content.Title)
	if item.Content.Number > 0 {
		return fmt.Sprintf("#%d %s", item.Content.Number, title)
	}
	return title
}

// renderDateTimeline preserves caller order and selection by ID. It only formats
// explicitly supplied data; it has no source, commands, mutations, or Model path.
func renderDateTimeline(rows []dateTimelineRow, view dateTimelineViewport) string {
	if view.Width <= 0 || view.Height <= 0 {
		return ""
	}
	fit := func(value string) string { return ansi.Truncate(value, view.Width, "…") }
	gutter := min(40, max(24, view.Width/3))
	calendar, err := newTimelineCalendar(view.Month, max(0, view.Width-gutter-5))
	if err != nil {
		return fit("Timeline unavailable: " + err.Error())
	}
	selected := 0
	outside, undated, unavailable, invalid := 0, 0, 0, 0
	for i, row := range rows {
		if row.Item.ID == view.SelectedID {
			selected = i
		}
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
	notice := "Local 3 months · proposed bars/points · " + calendar.start.Format("2006-01") + "–" + calendar.end.AddDate(0, 0, -1).Format("2006-01")
	if view.Width < 60 {
		notice = "Date list (narrow terminal) · proposed placement"
	}
	counts := fmt.Sprintf("Loaded %d · outside %d · undated %d · unavailable %d · invalid %d", len(rows), outside, undated, unavailable, invalid)
	if view.Loading {
		counts += " · loading"
	}
	headers := []string{notice, counts}
	if view.Width >= 60 {
		headers = append(headers, "  "+padANSI("Item", gutter)+" | "+calendar.header())
	}
	if len(rows) == 0 {
		headers = append(headers, "No loaded items")
		lines := headers[:min(len(headers), view.Height)]
		for i := range lines {
			lines[i] = fit(lines[i])
		}
		return strings.Join(lines, "\n")
	}
	footer := []string{"Start: " + timelineEndpointText(rows[selected].Start), "Target: " + timelineEndpointText(rows[selected].Target)}
	footer = footer[:min(2, view.Height-1)]
	headers = headers[:min(len(headers), max(0, view.Height-len(footer)-1))]
	capacity := view.Height - len(headers) - len(footer)
	start := max(0, min(selected-capacity/2, len(rows)-capacity))
	end := min(len(rows), start+capacity)
	lines := make([]string, 0, len(headers)+end-start+len(footer))
	for _, header := range headers {
		lines = append(lines, fit(header))
	}
	for i := start; i < end; i++ {
		marker := "  "
		if i == selected {
			marker = "> "
		}
		title := timelineRowTitle(rows[i].Item)
		span := timelineSpan(rows[i], calendar)
		var line string
		if view.Width < 60 {
			line = marker + title + " | " + span.status + " | " + timelineEndpointText(rows[i].Start) + " → " + timelineEndpointText(rows[i].Target)
		} else {
			line = marker + padANSI(ansi.Truncate(title, gutter, "…"), gutter) + " | " + calendar.bar(span)
		}
		lines = append(lines, fit(line))
	}
	for _, line := range footer {
		lines = append(lines, fit(line))
	}
	return strings.Join(lines, "\n")
}
