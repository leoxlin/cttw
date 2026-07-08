package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/strutil"
)

var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))

func dashboardView(m *Model) string {
	s := titleStyle.Render("cttw — Claudivicus Take The Wheel") + "\n\n"
	if m.Notice != "" {
		s += m.Notice + "\n\n"
	}
	if m.Err != nil {
		s += fmt.Sprintf("Error: %v\n\n", m.Err)
	}
	if len(m.Problems) == 0 {
		s += "No problems yet.\n\n"
	} else {
		s += "Problems:\n"
		for i, p := range m.Problems {
			marker := " "
			if i == m.Cursor {
				marker = ">"
			}
			line := fmt.Sprintf("%s %s  %-12s  %s", marker, strutil.ShortID(p.ID), p.Status, p.Description)
			s += truncate(line, 78) + "\n"
		}
		s += "\n"
	}
	s += "Keys: [n] new problem  [e] edit selected  [j/k] move  [q] quit  [esc] refresh\n"
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
