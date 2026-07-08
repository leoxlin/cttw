package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/strutil"
)

var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))

func dashboardView(m *Model) string {
	s := titleStyle.Render("cttw — Claudivicus Take The Wheel") + "\n\n"
	switch {
	case m.Loading:
		s += "Loading problems from daemon...\n\n"
	case m.Err != nil:
		s += requestErrorView(m.Err) + "\n\n"
	case len(m.Problems) == 0:
		s += "No problems yet.\n\n"
	default:
		s += "Problems:\n"
		for _, p := range m.Problems {
			line := fmt.Sprintf("  %s  %-12s  %s", strutil.ShortID(p.ID), p.Status, p.Description)
			s += truncate(line, 78) + "\n"
		}
		s += "\n"
	}
	s += "Keys: [n] new problem  [q] quit  [esc] refresh\n"
	return s
}

func requestErrorView(err error) string {
	if api.IsDisconnected(err) {
		return "Daemon disconnected.\nStart it with `cttw daemon start`, then press esc to refresh.\nDetails: " + err.Error()
	}

	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) {
		body := strings.TrimSpace(httpErr.Body)
		if body == "" {
			body = "The daemon returned an empty error response."
		}
		return fmt.Sprintf("Daemon API error (%d).\n%s", httpErr.StatusCode, body)
	}

	return "Error: " + err.Error()
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
