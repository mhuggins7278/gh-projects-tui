package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func (m Model) renderDetailPanel() string {
	frame := detailFrameStyle.Width(m.detailPanelWidth())
	if m.detailVisible {
		frame = frame.Height(m.detailPanelHeight())
	}
	if m.detailLoading {
		return frame.Render(sectionStyle.Render("Loading item details..."))
	}
	if m.detailErr != nil {
		body := errorStyle.Render("Detail failed: "+m.detailErr.Error()) + "\n" + mutedStyle.Render("Press esc to return to the board.")
		return frame.Render(body)
	}
	if m.detail == nil {
		return frame.Render(mutedStyle.Render("No item selected."))
	}
	if m.commentEditing {
		target := "issue"
		if m.detail.Content.Number > 0 {
			target = fmt.Sprintf("issue #%d", m.detail.Content.Number)
		}
		return frame.Render(sectionStyle.Render("New comment on "+target) + "\n" + truncateText(m.detail.Content.Title, m.detailPanelWidth()-4) + "\n\n" + m.commentDraft + "_\n\n" + mutedStyle.Render("enter submit · esc cancel"))
	}
	if m.issueActionConfirm {
		action := "Reopen"
		if m.issueActionClosing {
			action = "Close"
		}
		return frame.Render(sectionStyle.Render(fmt.Sprintf("%s issue #%d?", action, m.detail.Content.Number)) + "\n" + truncateText(m.detail.Content.Title, m.detailPanelWidth()-4) + "\n\n" + mutedStyle.Render("enter confirm · esc cancel"))
	}

	lines := detailLines(*m.detail, m.detailPanelWidth(), m.detailShowAll)
	viewport := m.detailViewportLines()
	start := 0
	var end int
	for {
		// Reapply the requested offset after reserving space for scroll markers.
		// The usable viewport can shrink when a marker is needed, which changes
		// the maximum valid start position by a line or two.
		start = m.detailOffset
		if start < 0 {
			start = 0
		}
		maxStart := len(lines) - viewport
		if maxStart < 0 {
			maxStart = 0
		}
		if start > maxStart {
			start = maxStart
		}
		end = start + viewport
		if end > len(lines) {
			end = len(lines)
		}
		markers := 0
		if start > 0 {
			markers++
		}
		if end < len(lines) {
			markers++
		}
		available := m.bodyCanvasHeight() - 4 - markers
		if available >= viewport || viewport <= 6 {
			break
		}
		viewport = available
	}
	visible := append([]string(nil), lines[start:end]...)
	if start > 0 {
		visible = append([]string{mutedStyle.Render("... scroll up for more")}, visible...)
	}
	if end < len(lines) {
		visible = append(visible, mutedStyle.Render("... scroll down for more"))
	}
	return frame.Render(strings.Join(visible, "\n"))
}

func (m Model) detailPanelWidth() int {
	if m.detailVisible {
		if m.wideDetailLayout() {
			return m.detailPaneOuterWidth() - 4
		}
		return m.frameContentWidth() - 4
	}
	width := m.frameContentWidth() - 8
	if width > 108 {
		width = 108
	}
	if width < 24 {
		width = m.frameContentWidth() - 4
	}
	if width < 16 {
		return 16
	}
	return width
}

func (m Model) wideDetailLayout() bool {
	return m.detailVisible && m.width >= 128
}

func (m Model) detailPaneOuterWidth() int {
	width := m.frameContentWidth() / 3
	if width < 42 {
		return 42
	}
	if width > 58 {
		return 58
	}
	return width
}

func (m Model) detailPanelHeight() int {
	height := m.bodyCanvasHeight() - 4
	if height < 4 {
		return 4
	}
	return height
}

func (m Model) detailViewportLines() int {
	lines := m.bodyCanvasHeight() - 4
	if lines < 6 {
		return 6
	}
	return lines
}

func (m Model) detailMaxOffset() int {
	if m.detail == nil {
		return 0
	}
	lineCount := len(detailLines(*m.detail, m.detailPanelWidth(), m.detailShowAll))
	viewport := m.detailViewportLines()
	if lineCount <= viewport {
		return 0
	}
	if viewport > 6 {
		viewport--
	}
	return lineCount - viewport
}

func (m *Model) moveDetail(delta int) {
	if m.detail == nil {
		return
	}
	max := m.detailMaxOffset()
	if m.detailOffset > max {
		m.detailOffset = max
	}
	m.detailOffset += delta
	if m.detailOffset < 0 {
		m.detailOffset = 0
	}
	if m.detailOffset > max {
		m.detailOffset = max
	}
}

func renderPopover(base, popup string, width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	baseLines := strings.Split(base, "\n")
	popupLines := strings.Split(popup, "\n")
	popupWidth := 0
	for _, line := range popupLines {
		if lineWidth := ansi.StringWidth(line); lineWidth > popupWidth {
			popupWidth = lineWidth
		}
	}
	if popupWidth > width {
		popupWidth = width
	}
	if len(popupLines) > height {
		popupLines = popupLines[:height]
	}
	x := (width - popupWidth) / 2
	y := (height - len(popupLines)) / 2
	if y < 0 {
		y = 0
	}
	lines := make([]string, height)
	for row := 0; row < height; row++ {
		baseLine := ""
		if row < len(baseLines) {
			baseLine = baseLines[row]
		}
		if row < y || row >= y+len(popupLines) {
			lines[row] = padANSI(baseLine, width)
			continue
		}
		popupLine := ansi.Truncate(popupLines[row-y], popupWidth, "")
		popupLine = padANSI(popupLine, popupWidth)
		left := ansi.Cut(baseLine, 0, x)
		left = padANSI(left, x)
		right := ansi.Cut(baseLine, x+popupWidth, width)
		right = padANSI(right, width-x-popupWidth)
		lines[row] = left + popupLine + right
	}
	return strings.Join(lines, "\n")
}

func padANSI(value string, width int) string {
	remaining := width - ansi.StringWidth(value)
	if remaining <= 0 {
		return value
	}
	return value + strings.Repeat(" ", remaining)
}

func detailLines(detail github.ItemDetail, width int, showAllFields bool) []string {
	lines := make([]string, 0, len(detail.Fields)+12)
	if detail.Content == nil {
		lines = append(lines, detailLabelStyle.Render("Content"), errorStyle.Render("Unavailable or deleted"))
		return lines
	}
	identity := contentKind(detail.Content)
	if detail.Content.Number > 0 {
		identity += fmt.Sprintf(" #%d", detail.Content.Number)
	}
	lines = append(lines,
		titleStyle.Render(truncateText(detail.Content.Title, width-4)),
		itemKindStyle(contentKind(detail.Content)).Render(identity),
	)
	if detail.Content.Kind == "Issue" && detail.Content.State != "" {
		lines = append(lines, "State: "+detail.Content.State)
	}
	if detail.Content.URL != "" {
		lines = append(lines, mutedStyle.Render(truncateText(detail.Content.URL, width-4)))
	}

	if pills := detailFieldPills(detail.Fields, width-4, showAllFields); len(pills) > 0 {
		propertyHeading := "Properties"
		if showAllFields {
			propertyHeading += " (all fields)"
		}
		lines = append(lines, "", detailLabelStyle.Render(propertyHeading))
		lines = append(lines, pills...)
	}

	lines = append(lines, "", sectionStyle.Render("Body"))
	if !detail.Content.BodyAvailable {
		lines = append(lines, statusStyle.Render("Body unavailable or inaccessible"))
	} else if strings.TrimSpace(detail.Content.Body) == "" {
		lines = append(lines, mutedStyle.Render("(empty body)"))
	} else {
		lines = append(lines, renderMarkdown(detail.Content.Body, width-4)...)
	}

	return lines
}

func detailFieldPills(fields []github.DetailField, width int, showAll bool) []string {
	if width < 12 {
		width = 12
	}
	lines := make([]string, 0, len(fields))
	current := ""
	for _, field := range fields {
		name := strings.TrimSpace(field.Field.Name)
		if name == "" {
			name = "Unnamed field"
		}
		if strings.EqualFold(name, "title") {
			continue
		}
		value, populated := detailFieldDisplay(field)
		unavailable := field.Value != nil && !field.Value.Available
		if !showAll && (!populated || unavailable) {
			continue
		}
		pillText := truncateText(name+": "+value, width-2)
		pill := detailFieldPillStyleFor(name, populated, unavailable).Render(pillText)
		if current == "" {
			current = pill
			continue
		}
		if ansi.StringWidth(current)+2+ansi.StringWidth(pill) <= width {
			current += "  " + pill
			continue
		}
		lines = append(lines, current)
		current = pill
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func detailFieldDisplay(field github.DetailField) (string, bool) {
	if field.Value == nil {
		return "No value", false
	}
	if !field.Value.Available {
		return "Unavailable", true
	}
	if strings.TrimSpace(field.Value.Value) == "" {
		return "No value", false
	}
	return field.Value.Value, true
}

func detailFieldPillStyleFor(name string, populated, unavailable bool) lipgloss.Style {
	if unavailable {
		return warningPillStyle
	}
	if !populated {
		return emptyPillStyle
	}
	switch strings.ToLower(name) {
	case "status", "state":
		return statusPillStyle
	case "priority":
		return priorityPillStyle
	default:
		return detailPillStyle
	}
}

func renderMarkdown(body string, width int) []string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
	)
	if err == nil {
		rendered, renderErr := renderer.Render(body)
		_ = renderer.Close()
		if renderErr == nil {
			rendered = strings.Trim(rendered, "\n")
			if rendered == "" {
				return []string{""}
			}
			return strings.Split(rendered, "\n")
		}
	}
	return renderMarkdownFallback(body, width)
}

func renderMarkdownFallback(body string, width int) []string {
	lines := make([]string, 0)
	inCode := false
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			for _, wrapped := range wrapText(line, width-2) {
				lines = append(lines, bodyCodeStyle.Render("  "+wrapped))
			}
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "#"):
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			for _, wrapped := range wrapText(heading, width) {
				lines = append(lines, sectionStyle.Render(wrapped))
			}
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			for index, wrapped := range wrapText(trimmed[2:], width-4) {
				prefix := "    "
				if index == 0 {
					prefix = "  - "
				}
				lines = append(lines, bodyTextStyle.Render(prefix+wrapped))
			}
		case strings.HasPrefix(trimmed, ">"):
			for _, wrapped := range wrapText(strings.TrimSpace(strings.TrimPrefix(trimmed, ">")), width-4) {
				lines = append(lines, bodyQuoteStyle.Render("  > "+wrapped))
			}
		case trimmed == "":
			lines = append(lines, "")
		default:
			for _, wrapped := range wrapText(trimmed, width) {
				lines = append(lines, bodyTextStyle.Render(wrapped))
			}
		}
	}
	return lines
}

func wrapText(value string, width int) []string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return []string{""}
	}
	if width < 1 {
		return []string{value}
	}
	words := strings.Fields(value)
	lines := make([]string, 0, len(words))
	line := ""
	for _, word := range words {
		for len([]rune(word)) > width {
			part := string([]rune(word)[:width])
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			lines = append(lines, part)
			word = string([]rune(word)[width:])
		}
		if line == "" {
			line = word
		} else if len([]rune(line))+1+len([]rune(word)) <= width {
			line += " " + word
		} else {
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
