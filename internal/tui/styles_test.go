package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestStatusLabels_RenderKnownStatuses(t *testing.T) {
	for _, label := range []string{
		problemStatusLabel("ready"),
		taskStatusLabel("running"),
	} {
		assert.Equal(t, statusLabelWidth, lipgloss.Width(label))
	}
}

func TestStatusLabels_RenderUnknownStatus(t *testing.T) {
	label := problemStatusLabel("waiting")

	assert.Contains(t, label, "waiting")
	assert.Equal(t, statusLabelWidth, lipgloss.Width(label))
}
