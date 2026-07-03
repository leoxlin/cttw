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
	if len(m.Tasks) == 0 {
		s += "No tasks yet.\n\n"
	} else {
		s += "Tasks:\n"
		for _, t := range m.Tasks {
			line := fmt.Sprintf("  %s  %-12s  %s", t.ID[:8], t.Status, t.Description)
			s += truncate(line, 78) + "\n"
		}
		s += "\n"
	}
	s += "Keys: [n] new task  [q] quit  [esc] refresh\n"
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
