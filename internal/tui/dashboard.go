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

var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))

func dashboardView(m *Model) string {
	s := titleStyle.Render("cttw — Claudivicus Take The Wheel") + "\n\n"
	s += fmt.Sprintf("Search: %s\n", searchDisplay(m))
	s += fmt.Sprintf("Sort: %s %s\n\n", sortLabel(m.Sort), sortDirection(m.SortDesc))

	if m.Loading {
		s += "Loading problems...\n\n"
		s += dashboardKeys(m)
		return s
	}

	if m.Err != nil {
		s += fmt.Sprintf("Error: %v\n\n", m.Err)
		s += dashboardKeys(m)
		return s
	}

	problems := visibleProblems(m)
	if len(problems) == 0 {
		if strings.TrimSpace(m.Search) != "" && len(m.Problems) > 0 {
			s += "No matching problems.\n\n"
		} else {
			s += "No problems yet.\n\n"
		}
	} else {
		s += fmt.Sprintf("Problems (%d", len(problems))
		if len(problems) != len(m.Problems) {
			s += fmt.Sprintf(" of %d", len(m.Problems))
		}
		s += "):\n"
		s += "  ID        STATUS        ISSUE   UPDATED     DESCRIPTION\n"
		for _, p := range problems {
			line := fmt.Sprintf("  %-8s  %-12s  %-6s  %-10s  %s",
				strutil.ShortID(p.ID),
				p.Status,
				issueLabel(p.IssueNumber),
				dateLabel(p.UpdatedAt),
				p.Description,
			)
			s += truncate(line, 100) + "\n"
		}
		s += "\n"
	}
	s += dashboardKeys(m)
	return s
}

func dashboardKeys(m *Model) string {
	searchKey := "[/] search"
	if m.searching {
		searchKey = "[enter] finish search"
	}
	return fmt.Sprintf("Keys: [n] new problem  %s  [ctrl+u] clear search  [s] sort  [a] direction  [esc] refresh  [q] quit\n", searchKey)
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
