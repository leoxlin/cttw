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
	s += "Keys: [up/down] move  [enter] details  [n] new problem  [q] quit  [esc] refresh\n"
	return s
}

func detailView(m *Model) string {
	s := titleStyle.Render("Problem Detail") + "\n\n"
	if m.Err != nil {
		s += fmt.Sprintf("Error: %v\n\n", m.Err)
		s += "Keys: [esc] back  [q] quit\n"
		return s
	}
	if m.Detail == nil {
		s += "Loading problem...\n\n"
		s += "Keys: [esc] back  [q] quit\n"
		return s
	}

	p := m.Detail
	s += fmt.Sprintf("%s  %s\n", strutil.ShortID(p.ID), p.Status)
	s += p.Description + "\n"
	if p.IssueNumber != 0 {
		s += fmt.Sprintf("Issue: #%d\n", p.IssueNumber)
	}
	if len(p.Tasks) > 0 {
		s += "\nTasks:\n"
		for _, task := range p.Tasks {
			line := fmt.Sprintf("  %s  %-12s  %s", strutil.ShortID(task.ID), task.Status, task.Title)
			s += truncate(line, 78) + "\n"
		}
	}
	s += "\nKeys: [esc] back  [q] quit\n"
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
