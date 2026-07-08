package tui

type ScopeDefinition struct {
	Workflows []WorkflowScope
	Screens   []ScreenScope
	Nav       []NavigationScope
	States    []StateScope
}

type WorkflowScope struct {
	Name   string
	Steps  []string
	Result string
}

type ScreenScope struct {
	ID      string
	Title   string
	Initial bool
	Role    string
}

type NavigationScope struct {
	From   string
	Key    string
	To     string
	Result string
}

type StateScope struct {
	Screen string
	Name   string
	Role   string
}

func Scope() ScopeDefinition {
	return ScopeDefinition{
		Workflows: []WorkflowScope{
			{
				Name: "Review problem queue",
				Steps: []string{
					"Open the TUI",
					"Scan problems by short ID, status, and description",
					"Refresh the queue after returning from creation",
				},
				Result: "User understands which problems are pending, ready, running, completed, or failed.",
			},
			{
				Name: "Create problem",
				Steps: []string{
					"Open the new-problem screen",
					"Enter owner/repo followed by a problem description",
					"Submit to the daemon and return to the dashboard",
				},
				Result: "Daemon owns decomposition into tasks and the refreshed dashboard shows the new problem.",
			},
		},
		Screens: []ScreenScope{
			{
				ID:      ScreenDashboard,
				Title:   "Dashboard",
				Initial: true,
				Role:    "Primary problem queue with creation and refresh entry points.",
			},
			{
				ID:    ScreenNewProblem,
				Title: "New Problem",
				Role:  "Problem intake form for selecting a repository and describing work.",
			},
		},
		Nav: []NavigationScope{
			{From: ScreenDashboard, Key: "n", To: ScreenNewProblem, Result: "Start problem creation."},
			{From: ScreenDashboard, Key: "esc", To: ScreenDashboard, Result: "Refresh problem list."},
			{From: ScreenDashboard, Key: "q", To: "", Result: "Quit."},
			{From: ScreenNewProblem, Key: "ctrl+d", To: ScreenDashboard, Result: "Submit problem and refresh on success."},
			{From: ScreenNewProblem, Key: "esc", To: ScreenDashboard, Result: "Cancel creation and refresh."},
		},
		States: []StateScope{
			{Screen: ScreenDashboard, Name: "loading", Role: "Initial fetch is in flight."},
			{Screen: ScreenDashboard, Name: "empty", Role: "No problems have been created."},
			{Screen: ScreenDashboard, Name: "populated", Role: "Problems are listed newest first."},
			{Screen: ScreenDashboard, Name: "error", Role: "Daemon or API fetch failed."},
			{Screen: ScreenNewProblem, Name: "editing", Role: "Textarea accepts owner/repo and description."},
			{Screen: ScreenNewProblem, Name: "submitting", Role: "Submission is in flight."},
			{Screen: ScreenNewProblem, Name: "validation_error", Role: "Input does not match owner/repo description."},
			{Screen: ScreenNewProblem, Name: "submit_error", Role: "Daemon or API create failed."},
			{Screen: ScreenNewProblem, Name: "created", Role: "Problem was accepted before returning to dashboard."},
		},
	}
}
