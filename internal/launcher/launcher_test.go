package launcher

import (
	"context"
	"testing"

	"github.com/llin/cttw/internal/acp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockLauncher_LaunchAndPrompt(t *testing.T) {
	ml := &MockLauncher{}
	ml.OnLaunch = func(spec LaunchSpec) (*MockAgent, error) {
		return &MockAgent{
			Responses: []string{"decomposed"},
		}, nil
	}

	agent, err := ml.Launch(context.Background(), LaunchSpec{
		Backend: "mock",
		Repo:    RepoContext{Owner: "llin", Name: "cttw", DefaultBranch: "main", LocalDir: "/tmp/r"},
		Task:    TaskContext{ProblemDescription: "build API", TaskTitle: "add handler", TaskDescription: "implement POST"},
	})
	require.NoError(t, err)
	require.NoError(t, agent.Initialize(context.Background()))
	require.NoError(t, agent.NewSession(context.Background(), acp.NewSessionRequest{CWD: "/tmp/r"}))

	res, err := agent.Prompt(context.Background(), "decompose this task")
	require.NoError(t, err)
	assert.Equal(t, "decomposed", res.Content)
	assert.Equal(t, "end_turn", res.StopReason)
	assert.Equal(t, "decompose this task", ml.Agent.PromptsReceived[0])

	require.NoError(t, agent.Close(context.Background()))
}
