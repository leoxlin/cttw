package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/strutil"
)

var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))

const (
	defaultWidth  = 80
	defaultHeight = 24
)

func dashboardView(m *Model) string {
	width := responsiveWidth(m.Width)

	s := titleStyle.Render(truncate("cttw — Claudivicus Take The Wheel", width)) + "\n\n"
	if m.Err != nil {
		s += truncate(fmt.Sprintf("Error: %v", m.Err), width) + "\n\n"
	}
	if len(m.Problems) == 0 {
		s += truncate("No problems yet.", width) + "\n\n"
	} else {
		s += truncate("Problems:", width) + "\n"
		for _, p := range m.Problems {
			line := fmt.Sprintf("  %s  %-12s  %s", strutil.ShortID(p.ID), p.Status, p.Description)
			s += truncate(line, width) + "\n"
		}
		s += "\n"
	}
	s += helpView(width)
	return s
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	const tail = "..."
	if n <= len(tail) {
		return strings.Repeat(".", n)
	}
	limit := n - len(tail)
	var b strings.Builder
	for _, r := range s {
		next := b.String() + string(r)
		if lipgloss.Width(next) > limit {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + tail
}

func helpView(width int) string {
	if width >= 46 {
		return truncate("Keys: [n] new problem  [q] quit  [esc] refresh", width) + "\n"
	}
	lines := []string{
		"Keys:",
		"  [n] new problem",
		"  [q] quit  [esc] refresh",
	}
	for i, line := range lines {
		lines[i] = truncate(line, width)
	}
	return strings.Join(lines, "\n") + "\n"
}

func responsiveWidth(width int) int {
	if width <= 0 {
		return defaultWidth
	}
	return width
}
