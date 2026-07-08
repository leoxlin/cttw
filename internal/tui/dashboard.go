package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/strutil"
)

func dashboardView(m *Model, width int) string {
	var lines []string
	lines = append(lines, sectionTitleStyle.Render("Problems"), "")
	lines = append(lines,
		fmt.Sprintf("Search: %s", searchDisplay(m)),
		fmt.Sprintf("Sort: %s %s", sortLabel(m.Sort), sortDirection(m.SortDesc)),
		"",
	)
	if m.Notice != "" {
		lines = append(lines, helpStyle.Render(m.Notice), "")
	}

	if m.Loading {
		lines = append(lines, mutedStyle.Render("Loading problems..."), "", helpStyle.Render(dashboardKeysForWidth(m, width)))
		return strings.Join(truncateLines(lines, width), "\n")
	}

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
		"[e]   Edit problem    update selected problem",
		"[/]   Search          filter visible problems",
		"[j/k] Select          move through problems",
		"[enter] Details       open selected problem",
		"[s]   Sort            change sort field",
		"[a]   Direction       toggle sort direction",
		"[esc] Refresh         reload daemon data",
		"[q]   Quit            leave cttw",
		"",
		sectionTitleStyle.Render("Recent Problems"),
	)

	problems := visibleProblems(m)
	if len(problems) == 0 {
		if strings.TrimSpace(m.Search) != "" && len(m.Problems) > 0 {
			lines = append(lines, mutedStyle.Render("No matching problems."), "")
		} else {
			lines = append(lines, mutedStyle.Render("No problems yet. Press [n] to create one."), "")
		}
	} else {
		count := fmt.Sprintf("Problems (%d", len(problems))
		if len(problems) != len(m.Problems) {
			count += fmt.Sprintf(" of %d", len(m.Problems))
		}
		count += ")"
		lines = append(lines, count, helpStyle.Render("  ID        Status       Issue   Updated     Description"))
		for i, p := range problems {
			marker := " "
			if i == m.Cursor {
				marker = ">"
			}
			taskSummary := ""
			if len(p.Tasks) > 0 {
				taskSummary = fmt.Sprintf("  %d tasks", len(p.Tasks))
			}
			line := fmt.Sprintf("%s %-8s  %-11s  %-6s  %-10s  %s%s",
				marker,
				strutil.ShortID(p.ID),
				p.Status,
				issueLabel(p.IssueNumber),
				dateLabel(p.UpdatedAt),
				p.Description,
				taskSummary,
			)
			lines = append(lines, truncate(line, width))
		}
		lines = append(lines, "")
	}
	lines = append(lines, helpStyle.Render(dashboardKeysForWidth(m, width)))
	return strings.Join(truncateLines(lines, width), "\n")
}

func detailView(m *Model) string {
	var lines []string
	lines = append(lines, sectionTitleStyle.Render("Problem Details"), "")
	if m.DetailErr != nil {
		lines = append(lines, errorStyle.Render(fmt.Sprintf("Error: %v", m.DetailErr)), "")
	}
	if m.Detail == nil {
		if m.DetailLoading {
			lines = append(lines, mutedStyle.Render("Loading problem..."), "")
		} else {
			lines = append(lines, mutedStyle.Render("No problem selected."), "")
		}
		lines = append(lines, helpStyle.Render("Actions: [b/esc] back  [q] quit"))
		return strings.Join(lines, "\n")
	}

	p := *m.Detail
	lines = append(lines,
		fmt.Sprintf("ID: %s", p.ID),
		fmt.Sprintf("Status: %s", p.Status),
		fmt.Sprintf("Repo: %s", p.RepoID),
		fmt.Sprintf("Issue: %s", issueLabel(p.IssueNumber)),
		fmt.Sprintf("Created: %s", formatTime(p.CreatedAt)),
		fmt.Sprintf("Updated: %s", formatTime(p.UpdatedAt)),
		"",
		"Description:",
		indentLines(p.Description, "  "),
		"",
		taskSummary(p.Tasks),
	)
	if m.DetailLoading {
		lines = append(lines, mutedStyle.Render("Refreshing..."))
	}
	lines = append(lines, helpStyle.Render("Actions: [r] refresh  [b/esc] back  [n] new problem  [q] quit"))
	return strings.Join(lines, "\n")
}

func taskSummary(tasks []api.TaskResponse) string {
	if len(tasks) == 0 {
		return "Tasks: none"
	}
	lines := []string{fmt.Sprintf("Tasks (%d):", len(tasks))}
	for _, t := range tasks {
		line := fmt.Sprintf("  %s  %-12s  %s", strutil.ShortID(t.ID), t.Status, t.Title)
		if t.PRNumber > 0 {
			line += fmt.Sprintf("  PR: #%d", t.PRNumber)
		}
		lines = append(lines, truncate(line, 78))
		if t.Description != "" {
			lines = append(lines, truncate("    "+t.Description, 78))
		}
	}
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

func dashboardKeys(m *Model) string {
	searchKey := "[/] search"
	if m.searching {
		searchKey = "[enter] finish search"
	}
	return fmt.Sprintf("Keys: [n] new problem  [e] edit  %s  [ctrl+u] clear search  [j/k] select  [enter] details  [s] sort  [a] direction  [esc] refresh  [q] quit", searchKey)
}

func dashboardKeysForWidth(m *Model, width int) string {
	if width >= 46 {
		return dashboardKeys(m)
	}
	searchKey := "[/] search"
	if m.searching {
		searchKey = "[enter] done"
	}
	return strings.Join([]string{
		"Keys:",
		"  [n] new  [e] edit",
		"  " + searchKey,
		"  [j/k] select",
		"  [enter] details",
		"  [q] quit  [esc] refresh",
	}, "\n")
}

func searchDisplay(m *Model) string {
	if m.Search == "" {
		if m.searching {
			return "_"
		}
		return "-"
	}
	if m.searching {
		return m.Search + "_"
	}
	return m.Search
}

func sortLabel(sort problemSort) string {
	switch sort {
	case sortStatus:
		return "status"
	case sortDescription:
		return "description"
	default:
		return "created"
	}
}

func sortDirection(desc bool) string {
	if desc {
		return "desc"
	}
	return "asc"
}

func visibleProblems(m *Model) []api.ProblemResponse {
	query := strings.ToLower(strings.TrimSpace(m.Search))
	problems := make([]api.ProblemResponse, 0, len(m.Problems))
	for _, p := range m.Problems {
		if query == "" || problemMatches(p, query) {
			problems = append(problems, p)
		}
	}
	sort.SliceStable(problems, func(i, j int) bool {
		return problemLess(problems[i], problems[j], m.Sort, m.SortDesc)
	})
	return problems
}

func problemMatches(p api.ProblemResponse, query string) bool {
	values := []string{
		p.ID,
		strutil.ShortID(p.ID),
		p.Description,
		p.Status,
		p.RepoID,
		strconv.Itoa(p.IssueNumber),
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func problemLess(a, b api.ProblemResponse, sort problemSort, desc bool) bool {
	cmp := compareProblems(a, b, sort)
	if cmp == 0 {
		cmp = strings.Compare(a.ID, b.ID)
	}
	if desc {
		return cmp > 0
	}
	return cmp < 0
}

func compareProblems(a, b api.ProblemResponse, sort problemSort) int {
	switch sort {
	case sortStatus:
		return strings.Compare(a.Status, b.Status)
	case sortDescription:
		return strings.Compare(strings.ToLower(a.Description), strings.ToLower(b.Description))
	default:
		return compareTimes(a.CreatedAt, b.CreatedAt)
	}
}

func compareTimes(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	default:
		return 0
	}
}

func issueLabel(issue int) string {
	if issue <= 0 {
		return "-"
	}
	return "#" + strconv.Itoa(issue)
}

func dateLabel(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
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

func truncateLines(lines []string, width int) []string {
	if width >= 46 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\n")
		for _, part := range parts {
			out = append(out, truncate(part, width))
		}
	}
	return out
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
