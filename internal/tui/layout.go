package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	defaultShellWidth  = 80
	defaultShellHeight = 24
	wideShellWidth     = 88
	sidebarWidth       = 22
)

type navItem struct {
	screen string
	key    string
	label  string
}

var navItems = []navItem{
	{screen: screenDashboard, key: "esc", label: "Problems"},
	{screen: screenNewTask, key: "n", label: "New problem"},
}

var (
	shellStyle = lipgloss.NewStyle().
			Padding(1, 2)

	brandStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E6EDF3"))

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8B949E"))

	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(lipgloss.Color("#30363D")).
			Padding(0, 2, 0, 0)

	pageStyle = lipgloss.NewStyle().
			Padding(0, 0, 0, 2)

	compactPageStyle = lipgloss.NewStyle().
				PaddingTop(1)

	navStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8B949E"))

	activeNavStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#58A6FF"))

	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#E6EDF3"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF7B72"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8B949E"))
)

func renderShell(m *Model, content string) string {
	width := maxInt(m.Width, 1)
	innerWidth := maxInt(width-shellStyle.GetHorizontalFrameSize(), 1)
	body := renderCompactShell(m, content, innerWidth)
	if width >= wideShellWidth {
		body = renderWideShell(m, content, innerWidth)
	}
	return shellStyle.Width(innerWidth).Render(body)
}

func renderWideShell(m *Model, content string, width int) string {
	sidebar := sidebarStyle.Width(sidebarWidth).Render(strings.Join([]string{
		brandStyle.Render("cttw"),
		mutedStyle.Render("Repository manager"),
		"",
		renderNav(m, true),
		"",
		helpStyle.Render("q quit"),
	}, "\n"))

	pageWidth := maxInt(width-lipgloss.Width(sidebar), 24)
	page := pageStyle.Width(pageWidth).Render(content)
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, page)
}

func renderCompactShell(m *Model, content string, width int) string {
	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		brandStyle.Render("cttw"),
		"  ",
		mutedStyle.Render("Repository manager"),
	)
	return strings.Join([]string{
		header,
		renderNav(m, false),
		compactPageStyle.Width(width).Render(content),
	}, "\n")
}

func renderNav(m *Model, vertical bool) string {
	parts := make([]string, 0, len(navItems))
	for _, item := range navItems {
		label := item.label
		if item.key != "" {
			label = "[" + item.key + "] " + label
		}
		style := navStyle
		if m.Screen == item.screen {
			style = activeNavStyle
		}
		parts = append(parts, style.Render(label))
	}
	if vertical {
		return strings.Join(parts, "\n")
	}
	return strings.Join(parts, "  ")
}

func contentWidth(shellWidth int) int {
	if shellWidth <= 0 {
		shellWidth = defaultShellWidth
	}
	width := shellWidth - shellStyle.GetHorizontalFrameSize()
	if shellWidth >= wideShellWidth {
		width -= sidebarWidth + sidebarStyle.GetHorizontalFrameSize() + pageStyle.GetHorizontalFrameSize()
	}
	return maxInt(width, 24)
}

func contentHeight(shellHeight int) int {
	if shellHeight <= 0 {
		shellHeight = defaultShellHeight
	}
	return maxInt(shellHeight-shellStyle.GetVerticalFrameSize()-6, 5)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
