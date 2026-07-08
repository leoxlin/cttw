package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/strutil"
)

var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))
var sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00A896"))
var headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#626262"))

func dashboardView(m *Model) string {
	s := titleStyle.Render("cttw — Claudivicus Take The Wheel") + "\n\n"
	if m.Err != nil {
		s += fmt.Sprintf("Error: %v\n\n", m.Err)
	}
	if len(m.Problems) == 0 {
		s += "No problems yet.\n\n"
	} else {
		s += sectionStyle.Render("Repositories") + "\n\n"
		for _, repo := range dashboardRepos(m.Problems) {
			s += sectionStyle.Render(fmt.Sprintf(
				"%s  %d %s  %d %s  updated %s",
				repo.name,
				len(repo.problems),
				plural("problem", len(repo.problems)),
				repo.taskCount,
				plural("task", repo.taskCount),
				formatUpdated(repo.updatedAt),
			)) + "\n"
			s += headerStyle.Render(fmt.Sprintf("  %-10s %-7s %-5s %-16s %s", "STATUS", "ISSUE", "TASKS", "UPDATED", "PROBLEM")) + "\n"
			for _, p := range repo.problems {
				line := fmt.Sprintf(
					"  %-10s %-7s %-5d %-16s %s",
					p.Status,
					formatIssue(p.IssueNumber),
					len(p.Tasks),
					formatUpdated(p.UpdatedAt),
					p.Description,
				)
				s += truncate(line, 110) + "\n"
			}
			s += "\n"
		}
	}
	s += "Keys: [n] new problem  [q] quit  [esc] refresh\n"
	return s
}

type dashboardRepo struct {
	name      string
	problems  []api.ProblemResponse
	taskCount int
	updatedAt time.Time
}

func dashboardRepos(problems []api.ProblemResponse) []dashboardRepo {
	byRepo := make(map[string]*dashboardRepo)
	for _, p := range problems {
		name := repoName(p)
		repo, ok := byRepo[name]
		if !ok {
			repo = &dashboardRepo{name: name}
			byRepo[name] = repo
		}
		repo.problems = append(repo.problems, p)
		repo.taskCount += len(p.Tasks)
		if p.UpdatedAt.After(repo.updatedAt) {
			repo.updatedAt = p.UpdatedAt
		}
	}

	repos := make([]dashboardRepo, 0, len(byRepo))
	for _, repo := range byRepo {
		repos = append(repos, *repo)
	}
	sort.Slice(repos, func(i, j int) bool {
		if repos[i].updatedAt.Equal(repos[j].updatedAt) {
			return repos[i].name < repos[j].name
		}
		return repos[i].updatedAt.After(repos[j].updatedAt)
	})
	return repos
}

func repoName(p api.ProblemResponse) string {
	if p.RepoOwner != "" && p.RepoName != "" {
		return p.RepoOwner + "/" + p.RepoName
	}
	if p.RepoID != "" {
		return "repo " + strutil.ShortID(p.RepoID)
	}
	return "unknown repo"
}

func formatIssue(n int) string {
	if n <= 0 {
		return "-"
	}
	return fmt.Sprintf("#%d", n)
}

func formatUpdated(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func plural(word string, count int) string {
	if count == 1 {
		return word
	}
	if strings.HasSuffix(word, "s") {
		return word + "es"
	}
	return word + "s"
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
