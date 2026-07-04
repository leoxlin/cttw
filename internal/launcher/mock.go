package launcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/llin/cttw/internal/acp"
)

// MockAgent records calls and returns scripted responses.
type MockAgent struct {
	mu              sync.Mutex
	Initialized     bool
	SessionCreated  bool
	Closed          bool
	PromptsReceived []string
	Responses       []string
	responseIndex   int
}

func (m *MockAgent) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Initialized = true
	return nil
}

func (m *MockAgent) NewSession(ctx context.Context, req acp.NewSessionRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SessionCreated = true
	return nil
}

func (m *MockAgent) Prompt(ctx context.Context, prompt string) (*acp.PromptResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PromptsReceived = append(m.PromptsReceived, prompt)
	if m.responseIndex >= len(m.Responses) {
		return nil, fmt.Errorf("no scripted response")
	}
	m.responseIndex++
	return &acp.PromptResponse{
		Content:    m.Responses[m.responseIndex-1],
		StopReason: acp.StopReasonEndTurn,
	}, nil
}

func (m *MockAgent) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Closed = true
	return nil
}

// MockLauncher returns scripted MockAgents.
type MockLauncher struct {
	mu       sync.Mutex
	Agent    *MockAgent
	OnLaunch func(spec LaunchSpec) (*MockAgent, error)
}

func (m *MockLauncher) Launch(ctx context.Context, spec LaunchSpec) (Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.OnLaunch == nil {
		m.Agent = &MockAgent{}
		return m.Agent, nil
	}
	agent, err := m.OnLaunch(spec)
	if err != nil {
		return nil, err
	}
	m.Agent = agent
	return agent, nil
}
