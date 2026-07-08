package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/strutil"
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
		for i, p := range m.Problems {
			cursor := " "
			if i == m.Cursor {
				cursor = ">"
			}
			line := fmt.Sprintf("%s %s  %-12s  %s", cursor, strutil.ShortID(p.ID), p.Status, p.Description)
			s += truncate(line, 78) + "\n"
		}
		s += "\n"
	}
	s += "Keys: [↑/k] up  [↓/j] down  [enter] select  [r] refresh  [n] new  [q] quit\n"
	return s
}

func problemView(m *Model) string {
	s := titleStyle.Render("Problem") + "\n\n"
	if m.Err != nil {
		s += fmt.Sprintf("Error: %v\n\n", m.Err)
	}
	if m.Problem == nil {
		s += "Loading...\n\n"
	} else {
		p := m.Problem
		s += fmt.Sprintf("ID: %s\n", p.ID)
		s += fmt.Sprintf("Status: %s\n", p.Status)
		s += fmt.Sprintf("Description: %s\n\n", p.Description)
		if len(p.Tasks) == 0 {
			s += "No tasks yet.\n\n"
		} else {
			s += "Tasks:\n"
			for _, t := range p.Tasks {
				line := fmt.Sprintf("  %s  %-12s  %s", strutil.ShortID(t.ID), t.Status, t.Title)
				s += truncate(line, 78) + "\n"
			}
			s += "\n"
		}
	}
	s += "Keys: [b/esc] back  [r] refresh  [n] new  [q] quit\n"
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
