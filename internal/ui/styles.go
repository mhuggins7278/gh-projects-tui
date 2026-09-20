package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	ink         = lipgloss.Color("#0d1117")
	panel       = lipgloss.Color("#161b22")
	panelRaised = lipgloss.Color("#21262d")
	border      = lipgloss.Color("#30363d")
	text        = lipgloss.Color("#c9d1d9")
	bright      = lipgloss.Color("#f0f6fc")
	muted       = lipgloss.Color("#8b949e")
	blue        = lipgloss.Color("#58a6ff")
	green       = lipgloss.Color("#3fb950")
	yellow      = lipgloss.Color("#d29922")
	red         = lipgloss.Color("#f85149")
	purple      = lipgloss.Color("#bc8cff")

	brandStyle   = lipgloss.NewStyle().Bold(true).Foreground(bright)
	hostStyle    = lipgloss.NewStyle().Foreground(muted)
	crumbStyle   = lipgloss.NewStyle().Foreground(blue)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(bright)
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(bright)
	errorStyle   = lipgloss.NewStyle().Foreground(red)
	statusStyle  = lipgloss.NewStyle().Foreground(yellow)

	headerFrameStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(border).
				Background(ink).
				Padding(0, 1)
	bodyFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Background(panel).
			Foreground(text).
			Padding(1, 1)
	footerFrameStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(border).
				Foreground(muted).
				Padding(0, 1)
	helpStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(blue).
			Foreground(text).
			Padding(0, 1)
	detailFrameStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(purple).
				Background(panelRaised).
				Foreground(text).
				Padding(1, 1)
	detailLabelStyle  = lipgloss.NewStyle().Foreground(muted).Bold(true)
	detailValueStyle  = lipgloss.NewStyle().Foreground(text)
	bodyTextStyle     = lipgloss.NewStyle().Foreground(bright)
	bodyCodeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#d2a8ff"))
	bodyQuoteStyle    = lipgloss.NewStyle().Foreground(text).Italic(true)
	detailPillStyle   = lipgloss.NewStyle().Foreground(text).Background(panel).Padding(0, 1)
	statusPillStyle   = lipgloss.NewStyle().Foreground(bright).Background(lipgloss.Color("#238636")).Padding(0, 1)
	priorityPillStyle = lipgloss.NewStyle().Foreground(bright).Background(lipgloss.Color("#6e40c9")).Padding(0, 1)
	warningPillStyle  = lipgloss.NewStyle().Foreground(bright).Background(lipgloss.Color("#9e6a03")).Padding(0, 1)
	emptyPillStyle    = lipgloss.NewStyle().Foreground(muted).Background(panel).Padding(0, 1)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(bright).
				Background(lipgloss.Color("#1f3a5f")).
				Padding(0, 1)
	rowStyle   = lipgloss.NewStyle().Foreground(text).Padding(0, 1)
	badgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ink).
			Background(blue).
			Padding(0, 1)
	readOnlyBadgeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ink).
				Background(yellow).
				Padding(0, 1)
)

func (m Model) frameContentWidth() int {
	if m.width <= 0 {
		return 76
	}
	width := m.width - 6
	if width < 16 {
		return 16
	}
	return width
}

func (m Model) bodyCanvasHeight() int {
	if m.height <= 0 {
		return 32
	}
	height := m.height - 11
	if height < 8 {
		return 8
	}
	return height
}

func (m Model) renderHeader() string {
	mode := "READ ONLY"
	if m.screen == screenBoard && m.itemsLoading {
		mode = "SYNCING"
	}
	modeBadge := readOnlyBadgeStyle.Render(mode)
	top := lipgloss.JoinHorizontal(
		lipgloss.Center,
		brandStyle.Render("gh projects-tui"),
		"  ",
		hostStyle.Render("["+m.host+"]"),
		"  ",
		modeBadge,
	)
	return headerFrameStyle.Width(m.frameContentWidth()).Render(
		lipgloss.JoinVertical(lipgloss.Left, top, crumbStyle.Render(m.breadcrumb())),
	)
}

func (m Model) renderFooter() string {
	return footerFrameStyle.Width(m.frameContentWidth()).Render(m.footerHints())
}

func (m Model) renderScreen() string {
	content := &strings.Builder{}
	switch {
	case m.screen == screenLoading && m.loading:
		content.WriteString(sectionStyle.Render("Loading projects..."))
	case m.screen == screenLoading && !m.loading:
		// Legacy direct-state path used by tests: fall back to owner list.
		m.renderOwnerPicker(content)
	case m.screen == screenOwnerPicker:
		m.renderOwnerPicker(content)
	case m.screen == screenProjectPicker:
		m.renderProjectPicker(content)
	case m.screen == screenViewPicker:
		m.renderViewPicker(content)
	case m.screen == screenBoard:
		m.renderBoard(content)
	}

	if m.status != "" {
		content.WriteString("\n" + statusStyle.Render(m.status))
	}
	body := content.String()
	if m.showHelp {
		body = lipgloss.JoinVertical(lipgloss.Left, body, helpStyle.Width(m.frameContentWidth()-2).Render(m.helpText()))
	}
	return bodyFrameStyle.Width(m.frameContentWidth()).Render(body)
}

func (m Model) pickerHeading(title, hint string) string {
	return lipgloss.JoinHorizontal(lipgloss.Bottom, sectionStyle.Render(title), "  ", mutedStyle.Render(hint))
}

func (m Model) pickerRow(selected bool, text string) string {
	if selected {
		return selectedRowStyle.Render("> " + text)
	}
	return rowStyle.Render("  " + text)
}

func layoutBadge(layout string) string {
	if layout == string(boardLayoutValue) {
		return badgeStyle.Render("BOARD")
	}
	return readOnlyBadgeStyle.Render(strings.TrimSuffix(strings.TrimSuffix(layout, "_LAYOUT"), "_LAYOUT"))
}

// boardLayoutValue avoids importing the GitHub package into this style-only
// helper while keeping layout labels in one place.
const boardLayoutValue = "BOARD_LAYOUT"

func laneColor(index int) lipgloss.Color {
	colors := []lipgloss.Color{blue, purple, green, yellow, red}
	return colors[index%len(colors)]
}

func laneFrameStyle(index int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(laneColor(index)).
		Background(panel).
		Foreground(text).
		Padding(0, 1)
}

func laneTitleStyle(index int) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(laneColor(index))
}

func cardStyle(selected bool) lipgloss.Style {
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(border).
		Foreground(text).
		Padding(0, 1)
	if selected {
		return style.
			BorderForeground(blue).
			Background(lipgloss.Color("#1f3a5f")).
			Bold(true)
	}
	return style
}

func itemKindStyle(kind string) lipgloss.Style {
	switch kind {
	case "PR":
		return lipgloss.NewStyle().Foreground(purple).Bold(true)
	case "Draft":
		return lipgloss.NewStyle().Foreground(yellow).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(green).Bold(true)
	}
}

func truncateText(value string, width int) string {
	value = strings.Join(strings.Fields(value), " ")
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func itemCountLabel(count int) string {
	return fmt.Sprintf("%d", count)
}
