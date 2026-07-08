package tui

import (
	"errors"
	"net"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardView_Loading(t *testing.T) {
	m := &Model{Loading: true}

	view := dashboardView(m)

	assert.Contains(t, view, "Loading problems from daemon")
	assert.NotContains(t, view, "No problems yet")
}

func TestDashboardView_Empty(t *testing.T) {
	m := &Model{}

	view := dashboardView(m)

	assert.Contains(t, view, "No problems yet")
}

func TestDashboardView_DisconnectedDaemon(t *testing.T) {
	m := &Model{Err: &net.OpError{Op: "dial", Err: errors.New("connect: connection refused")}}

	view := dashboardView(m)

	assert.Contains(t, view, "Daemon disconnected")
	assert.Contains(t, view, "cttw daemon start")
	assert.NotContains(t, view, "No problems yet")
}

func TestDashboardView_APIError(t *testing.T) {
	m := &Model{Err: &api.HTTPError{StatusCode: 500, Body: "store failed\n"}}

	view := dashboardView(m)

	assert.Contains(t, view, "Daemon API error (500)")
	assert.Contains(t, view, "store failed")
	assert.NotContains(t, view, "No problems yet")
}

func TestModel_RefreshSetsLoading(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Err = errors.New("old")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nm := updated.(*Model)

	require.NotNil(t, cmd)
	assert.True(t, nm.Loading)
	assert.NoError(t, nm.Err)
}
