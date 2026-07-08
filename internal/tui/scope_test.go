package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeDefinesInitialScreensNavigationAndStates(t *testing.T) {
	scope := Scope()

	require.Len(t, scope.Workflows, 2)
	assert.Equal(t, "Review problem queue", scope.Workflows[0].Name)
	assert.Equal(t, "Create problem", scope.Workflows[1].Name)

	initialScreens := 0
	screenIDs := map[string]bool{}
	for _, screen := range scope.Screens {
		screenIDs[screen.ID] = true
		if screen.Initial {
			initialScreens++
			assert.Equal(t, ScreenDashboard, screen.ID)
		}
	}
	assert.Equal(t, 1, initialScreens)
	assert.True(t, screenIDs[ScreenDashboard])
	assert.True(t, screenIDs[ScreenNewProblem])

	assert.Contains(t, scope.Nav, NavigationScope{From: ScreenDashboard, Key: "n", To: ScreenNewProblem, Result: "Start problem creation."})
	assert.Contains(t, scope.Nav, NavigationScope{From: ScreenNewProblem, Key: "ctrl+d", To: ScreenDashboard, Result: "Submit problem and refresh on success."})

	stateNames := map[string]bool{}
	for _, state := range scope.States {
		stateNames[state.Screen+":"+state.Name] = true
	}
	for _, required := range []string{
		ScreenDashboard + ":loading",
		ScreenDashboard + ":empty",
		ScreenDashboard + ":populated",
		ScreenDashboard + ":error",
		ScreenNewProblem + ":editing",
		ScreenNewProblem + ":submitting",
		ScreenNewProblem + ":validation_error",
		ScreenNewProblem + ":submit_error",
		ScreenNewProblem + ":created",
	} {
		assert.True(t, stateNames[required], required)
	}
}

func TestNewModelStartsOnScopedInitialScreen(t *testing.T) {
	assert.Equal(t, ScreenDashboard, New("unix:///tmp/cttw.sock").Screen)
}
