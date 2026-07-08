package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const statusLabelWidth = 10

var statusLabelBaseStyle = lipgloss.NewStyle().
	Bold(true).
	Width(statusLabelWidth).
	Align(lipgloss.Center)

var problemStatusStyles = map[string]lipgloss.Style{
	"pending": statusLabelBaseStyle.Foreground(lipgloss.Color("#B45309")),
	"ready":   statusLabelBaseStyle.Foreground(lipgloss.Color("#047857")),
	"failed":  statusLabelBaseStyle.Foreground(lipgloss.Color("#DC2626")),
	"done":    statusLabelBaseStyle.Foreground(lipgloss.Color("#047857")),
}

var taskStatusStyles = map[string]lipgloss.Style{
	"pending":   statusLabelBaseStyle.Foreground(lipgloss.Color("#B45309")),
	"running":   statusLabelBaseStyle.Foreground(lipgloss.Color("#2563EB")),
	"completed": statusLabelBaseStyle.Foreground(lipgloss.Color("#047857")),
	"failed":    statusLabelBaseStyle.Foreground(lipgloss.Color("#DC2626")),
}

func problemStatusLabel(status string) string {
	return statusLabel(problemStatusStyles, status)
}

func taskStatusLabel(status string) string {
	return statusLabel(taskStatusStyles, status)
}

func statusLabel(styles map[string]lipgloss.Style, status string) string {
	key := strings.ToLower(strings.TrimSpace(status))
	style, ok := styles[key]
	if !ok {
		style = statusLabelBaseStyle.Inherit(mutedStyle)
	}
	return style.Render(status)
}
