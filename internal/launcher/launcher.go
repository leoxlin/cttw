package launcher

import (
	"context"

	"github.com/llin/cttw/internal/acp"
)

// LaunchSpec describes the context needed to launch an agent for a task.
type LaunchSpec struct {
	Backend string
	Repo    RepoContext
	Task    TaskContext
}

// RepoContext identifies the repository the agent will work in.
type RepoContext struct {
	Owner         string
	Name          string
	DefaultBranch string
	LocalDir      string
}

// TaskContext describes the problem and task the agent should solve.
type TaskContext struct {
	ProblemDescription string
	TaskTitle          string
	TaskDescription    string
	BaseBranch         string
}

// Agent is a handle to a running ACP agent.
type Agent interface {
	Initialize(ctx context.Context) error
	NewSession(ctx context.Context, req acp.NewSessionRequest) error
	SessionID() string
	Prompt(ctx context.Context, prompt string) (*acp.PromptResponse, error)
	Close(ctx context.Context) error
}

// Launcher starts an ACP agent from a LaunchSpec.
type Launcher interface {
	Launch(ctx context.Context, spec LaunchSpec) (Agent, error)
}
