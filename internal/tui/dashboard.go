package tui

import (
	"fmt"
	"strings"

	"github.com/llin/cttw/internal/strutil"
)

func dashboardView(m *Model, width int) string {
	var lines []string
	lines = append(lines, sectionTitleStyle.Render("Problems"), "")
	if m.Err != nil {
		lines = append(lines, errorStyle.Render(fmt.Sprintf("Error: %v", m.Err)), "")
	}
	if len(m.Problems) == 0 {
		lines = append(lines, mutedStyle.Render("No problems yet."), "")
	} else {
		lines = append(lines, helpStyle.Render("ID        Status       Description"))
		for _, p := range m.Problems {
			line := fmt.Sprintf("%-8s  %-11s  %s", strutil.ShortID(p.ID), p.Status, p.Description)
			lines = append(lines, truncate(line, width))
		}
		lines = append(lines, "")
	}
	lines = append(lines, helpStyle.Render("Keys: [n] new problem  [q] quit  [esc] refresh"))
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
