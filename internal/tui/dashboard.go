package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/api"
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
			marker := " "
			if i == m.Cursor {
				marker = ">"
			}
			line := fmt.Sprintf("%s %s  %-12s  %s", marker, strutil.ShortID(p.ID), p.Status, p.Description)
			s += truncate(line, 78) + "\n"
		}
		s += "\n"
	}
	s += "Keys: [j/k] select  [enter] details  [n] new problem  [r/esc] refresh  [q] quit\n"
	return s
}

func detailView(m *Model) string {
	s := titleStyle.Render("Problem Details") + "\n\n"
	if m.DetailErr != nil {
		s += fmt.Sprintf("Error: %v\n\n", m.DetailErr)
	}
	if m.Detail == nil {
		if m.DetailLoading {
			s += "Loading problem...\n\n"
		} else {
			s += "No problem selected.\n\n"
		}
		s += "Actions: [b/esc] back  [q] quit\n"
		return s
	}

	p := *m.Detail
	s += fmt.Sprintf("ID: %s\n", p.ID)
	s += fmt.Sprintf("Status: %s\n", p.Status)
	s += fmt.Sprintf("Repo: %s\n", p.RepoID)
	if p.IssueNumber > 0 {
		s += fmt.Sprintf("Issue: #%d\n", p.IssueNumber)
	} else {
		s += "Issue: none\n"
	}
	s += fmt.Sprintf("Created: %s\n", formatTime(p.CreatedAt))
	s += fmt.Sprintf("Updated: %s\n\n", formatTime(p.UpdatedAt))
	s += "Description:\n"
	s += indentLines(p.Description, "  ") + "\n\n"
	s += taskSummary(p.Tasks)
	if m.DetailLoading {
		s += "\nRefreshing...\n"
	}
	s += "\nActions: [r] refresh  [b/esc] back  [n] new problem  [q] quit\n"
	return s
}

func taskSummary(tasks []api.TaskResponse) string {
	if len(tasks) == 0 {
		return "Tasks: none\n"
	}
	s := fmt.Sprintf("Tasks (%d):\n", len(tasks))
	for _, t := range tasks {
		line := fmt.Sprintf("  %s  %-12s  %s", strutil.ShortID(t.ID), t.Status, t.Title)
		if t.PRNumber > 0 {
			line += fmt.Sprintf("  PR: #%d", t.PRNumber)
		}
		s += truncate(line, 78) + "\n"
		if t.Description != "" {
			s += truncate("    "+t.Description, 78) + "\n"
		}
	}
	return s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format("2006-01-02 15:04:05 MST")
}

func indentLines(s, prefix string) string {
	if s == "" {
		return prefix + "(empty)"
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
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
