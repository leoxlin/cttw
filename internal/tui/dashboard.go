package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/strutil"
)

func dashboardView(m *Model) string {
	s := headerStyle.Render("cttw — Claudivicus Take The Wheel") + "\n\n"
	if m.Err != nil {
		s += errorStyle.Render(fmt.Sprintf("Error: %v", m.Err)) + "\n\n"
	}
	if len(m.Problems) == 0 {
		s += mutedStyle.Render("No problems yet.") + "\n\n"
	} else {
		s += headerStyle.Render("Problems") + "\n"
		for _, p := range m.Problems {
			s += problemRow(p) + "\n"
		}
		s += "\n"
	}
	s += mutedStyle.Render("Keys: [n] new problem  [q] quit  [esc] refresh") + "\n"
	return s
}

func problemRow(p api.ProblemResponse) string {
	id := mutedStyle.Render(strutil.ShortID(p.ID))
	status := problemStatusLabel(p.Status)
	prefix := fmt.Sprintf("  %s  %s  ", id, status)
	description := truncate(p.Description, 78-lipgloss.Width(prefix))
	return renderRow(prefix+description, false)
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
