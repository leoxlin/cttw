package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
)

func TestDashboardView_RespectsNarrowWidth(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Width = 32
	m.Problems = []api.ProblemResponse{
		{
			ID:          "1234567890abcdef",
			Status:      "in_progress",
			Description: "implement a very long responsive terminal layout",
		},
	}

	view := dashboardView(m)

	assertMaxLineWidth(t, view, m.Width)
	assert.Contains(t, view, "Keys:\n")
}

func TestDashboardView_UsesWideHelpOnDesktopWidth(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Width = 80

	view := dashboardView(m)

	assertMaxLineWidth(t, view, m.Width)
	assert.Contains(t, view, "Keys: [n] new problem  [q] quit  [esc] refresh")
}

func assertMaxLineWidth(t *testing.T, view string, width int) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSuffix(view, "\n"), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), width, line)
	}
}
