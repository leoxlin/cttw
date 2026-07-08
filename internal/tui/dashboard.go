package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/strutil"
)

var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))

func dashboardView(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("cttw — Claudivicus Take The Wheel"))
	b.WriteString("\n\n")
	if m.Err != nil {
		fmt.Fprintf(&b, "Error: %v\n\n", m.Err)
	}

	stats := newDashboardStats(m.Problems)
	b.WriteString("Project Summary\n")
	fmt.Fprintf(&b, "  Problems: %d total, %d pending, %d ready, %d failed\n", stats.problems, stats.pendingProblems, stats.readyProblems, stats.failedProblems)
	fmt.Fprintf(&b, "  Repos:    %d tracked\n", stats.repos)
	if stats.tasks > 0 {
		fmt.Fprintf(&b, "  Tasks:    %d total, %d pending, %d running, %d completed, %d failed\n", stats.tasks, stats.pendingTasks, stats.runningTasks, stats.completedTasks, stats.failedTasks)
	}
	b.WriteString("\n")

	b.WriteString("Workflows\n")
	b.WriteString("  [n]   New problem     create work for a repository\n")
	b.WriteString("  [esc] Refresh         reload daemon data\n")
	b.WriteString("  [q]   Quit            leave cttw\n\n")

	if len(m.Problems) == 0 {
		b.WriteString("Recent Problems\n")
		b.WriteString("  No problems yet. Press [n] to create one.\n")
	} else {
		b.WriteString("Recent Problems\n")
		for _, p := range m.Problems {
			taskSummary := ""
			if len(p.Tasks) > 0 {
				taskSummary = fmt.Sprintf("  %d tasks", len(p.Tasks))
			}
			line := fmt.Sprintf("  %s  %-12s  %s%s", strutil.ShortID(p.ID), p.Status, p.Description, taskSummary)
			b.WriteString(truncate(line, 78))
			b.WriteString("\n")
		}
	}
	return b.String()
}

type dashboardStats struct {
	problems        int
	repos           int
	pendingProblems int
	readyProblems   int
	failedProblems  int
	tasks           int
	pendingTasks    int
	runningTasks    int
	completedTasks  int
	failedTasks     int
}

func newDashboardStats(problems []api.ProblemResponse) dashboardStats {
	stats := dashboardStats{problems: len(problems)}
	repos := make(map[string]struct{})
	for _, p := range problems {
		if p.RepoID != "" {
			repos[p.RepoID] = struct{}{}
		}
		switch p.Status {
		case "pending":
			stats.pendingProblems++
		case "ready":
			stats.readyProblems++
		case "failed":
			stats.failedProblems++
		}
		for _, t := range p.Tasks {
			stats.tasks++
			switch t.Status {
			case "pending":
				stats.pendingTasks++
			case "running":
				stats.runningTasks++
			case "completed":
				stats.completedTasks++
			case "failed":
				stats.failedTasks++
			}
		}
	}
	stats.repos = len(repos)
	return stats
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
