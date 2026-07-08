package tui

import (
	"fmt"
	"strings"

	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/strutil"
)

func dashboardView(m *Model, width int) string {
	var lines []string
	lines = append(lines, sectionTitleStyle.Render("Problems"), "")
	if m.Err != nil {
		lines = append(lines, errorStyle.Render(fmt.Sprintf("Error: %v", m.Err)), "")
	}

	stats := newDashboardStats(m.Problems)
	lines = append(lines,
		sectionTitleStyle.Render("Project Summary"),
		fmt.Sprintf("Problems: %d total, %d pending, %d ready, %d failed", stats.problems, stats.pendingProblems, stats.readyProblems, stats.failedProblems),
		fmt.Sprintf("Repos:    %d tracked", stats.repos),
	)
	if stats.tasks > 0 {
		lines = append(lines, fmt.Sprintf("Tasks:    %d total, %d pending, %d running, %d completed, %d failed", stats.tasks, stats.pendingTasks, stats.runningTasks, stats.completedTasks, stats.failedTasks))
	}
	lines = append(lines, "")

	lines = append(lines,
		sectionTitleStyle.Render("Workflows"),
		"[n]   New problem     create work for a repository",
		"[esc] Refresh         reload daemon data",
		"[q]   Quit            leave cttw",
		"",
		sectionTitleStyle.Render("Recent Problems"),
	)
	if len(m.Problems) == 0 {
		lines = append(lines, mutedStyle.Render("No problems yet. Press [n] to create one."), "")
	} else {
		lines = append(lines, helpStyle.Render("ID        Status       Description"))
		for _, p := range m.Problems {
			taskSummary := ""
			if len(p.Tasks) > 0 {
				taskSummary = fmt.Sprintf("  %d tasks", len(p.Tasks))
			}
			line := fmt.Sprintf("%-8s  %-11s  %s%s", strutil.ShortID(p.ID), p.Status, p.Description, taskSummary)
			lines = append(lines, truncate(line, width))
		}
		lines = append(lines, "")
	}
	lines = append(lines, helpStyle.Render("Keys: [n] new problem  [q] quit  [esc] refresh"))
	return strings.Join(lines, "\n")
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
