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
			cursor := " "
			if i == m.Cursor {
				cursor = ">"
			}
			issue := "Issue: none"
			if p.IssueNumber != 0 {
				issue = fmt.Sprintf("Issue: #%d", p.IssueNumber)
			}
			line := fmt.Sprintf("%s %s  %-12s  %-12s  %s", cursor, strutil.ShortID(p.ID), p.Status, issue, p.Description)
			s += truncate(line, 78) + "\n"
		}
		s += "\n"
	}
	s += "Keys: [j/k] select  [enter] details  [n] new problem  [q] quit  [esc] refresh\n"
	return s
}

func problemDetailView(m *Model) string {
	s := titleStyle.Render("cttw — Claudivicus Take The Wheel") + "\n\n"
	if m.ProblemLoading {
		s += "Loading problem...\n\n"
	}
	if m.ProblemErr != nil {
		s += fmt.Sprintf("Error: %v\n\n", m.ProblemErr)
		s += "Keys: [esc] back  [q] quit\n"
		return s
	}
	if m.Problem == nil {
		s += "No problem selected.\n\n"
		s += "Keys: [esc] back  [q] quit\n"
		return s
	}

	p := m.Problem
	s += fmt.Sprintf("%s  %s\n", strutil.ShortID(p.ID), p.Status)
	s += fmt.Sprintf("Description: %s\n", p.Description)
	s += fmt.Sprintf("Repo: %s\n", p.RepoID)
	if p.IssueNumber != 0 {
		s += fmt.Sprintf("Issue: #%d\n", p.IssueNumber)
	} else {
		s += "Issue: none\n"
	}
	s += fmt.Sprintf("Created: %s\n", formatTime(p.CreatedAt))
	s += fmt.Sprintf("Updated: %s\n\n", formatTime(p.UpdatedAt))

	s += "Tasks:\n"
	if len(p.Tasks) == 0 {
		s += "  No tasks yet.\n"
	} else {
		for _, task := range p.Tasks {
			title := task.Title
			if title == "" {
				title = strutil.ShortID(task.ID)
			}
			line := fmt.Sprintf("  %s  %s", title, taskMetadata(task))
			s += truncate(line, 78) + "\n"
			if task.Description != "" {
				s += truncate("    "+task.Description, 78) + "\n"
			}
		}
	}
	s += "\nKeys: [esc] back  [q] quit\n"
	return s
}

func taskMetadata(task api.TaskResponse) string {
	issue := "Issue: none"
	if task.IssueNumber != 0 {
		issue = fmt.Sprintf("Issue: #%d", task.IssueNumber)
	}
	pr := "PR: none"
	if task.PRNumber != 0 {
		pr = fmt.Sprintf("PR: #%d", task.PRNumber)
	}
	return strings.Join([]string{task.Status, issue, pr}, "  ")
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format("2006-01-02 15:04 MST")
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
