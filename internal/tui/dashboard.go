package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))

func dashboardView(m *Model) string {
	s := titleStyle.Render("cttw — Claudivicus Take The Wheel") + "\n\n"
	if m.Err != nil {
		s += fmt.Sprintf("Error: %v\n\n", m.Err)
	}
	if len(m.Problems) == 0 {
		s += "No problems yet.\n\n"
	} else {
		s += "Problems:\n"
		for _, p := range m.Problems {
			line := fmt.Sprintf("  %s  %-12s  %s", p.ID[:8], p.Status, p.Description)
			s += truncate(line, 78) + "\n"
		}
		s += "\n"
	}
	s += "Keys: [n] new problem  [q] quit  [esc] refresh\n"
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
