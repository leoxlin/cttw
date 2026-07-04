package launcher

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/llin/cttw/internal/acp"
	"github.com/llin/cttw/internal/config"
)

// CodexLauncher spawns a local codex-acp process and wraps it as an Agent.
type CodexLauncher struct {
	cfg *config.Config
}

func NewCodexLauncher(cfg *config.Config) Launcher {
	return &CodexLauncher{cfg: cfg}
}

func (l *CodexLauncher) Launch(ctx context.Context, spec LaunchSpec) (Agent, error) {
	backend, ok := l.cfg.Agent.Backends[spec.Backend]
	if !ok {
		return nil, fmt.Errorf("backend %q not configured", spec.Backend)
	}
	if backend.Type != "local" {
		return nil, fmt.Errorf("backend %q has unsupported type %q", spec.Backend, backend.Type)
	}
	if backend.Command == "" {
		return nil, fmt.Errorf("backend %q missing command", spec.Backend)
	}

	cmd := exec.CommandContext(ctx, backend.Command)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	transport := acp.NewStdioTransport(stdin, stdout)
	client := acp.NewClient(transport)
	client.SetHandler(&defaultHandler{})

	go func() {
		_ = client.Start(ctx)
	}()

	return &codexAgent{
		client: client,
		cmd:    cmd,
		spec:   spec,
	}, nil
}

type codexAgent struct {
	client *acp.Client
	cmd    *exec.Cmd
	spec   LaunchSpec
}

func (a *codexAgent) Initialize(ctx context.Context) error {
	_, err := a.client.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo:      acp.Info{Name: "cttw", Version: "0.1"},
	})
	return err
}

func (a *codexAgent) NewSession(ctx context.Context, req acp.NewSessionRequest) error {
	if req.CWD == "" {
		req.CWD = a.spec.Repo.LocalDir
	}
	_, err := a.client.NewSession(ctx, req)
	return err
}

func (a *codexAgent) Prompt(ctx context.Context, prompt string) (*acp.PromptResponse, error) {
	return a.client.Prompt(ctx, acp.PromptRequest{
		SessionID: "session",
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
}

func (a *codexAgent) Close(ctx context.Context) error {
	_ = a.client.CloseSession(ctx, acp.CloseSessionRequest{SessionID: "session"})
	return a.client.Close()
}

// defaultHandler provides conservative defaults for agent-to-client methods.
type defaultHandler struct{}

func (d *defaultHandler) HandleReadTextFile(ctx context.Context, req acp.ReadTextFileRequest) (*acp.ReadTextFileResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *defaultHandler) HandleWriteTextFile(ctx context.Context, req acp.WriteTextFileRequest) error {
	return fmt.Errorf("not implemented")
}

func (d *defaultHandler) HandleCreateTerminal(ctx context.Context, req acp.CreateTerminalRequest) (*acp.CreateTerminalResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *defaultHandler) HandleTerminalOutput(ctx context.Context, req acp.TerminalOutputRequest) (*acp.TerminalOutputResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *defaultHandler) HandleWaitForTerminalExit(ctx context.Context, req acp.WaitForTerminalExitRequest) (*acp.WaitForTerminalExitResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *defaultHandler) HandleReleaseTerminal(ctx context.Context, req acp.ReleaseTerminalRequest) error {
	return fmt.Errorf("not implemented")
}

func (d *defaultHandler) HandleRequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (*acp.RequestPermissionResponse, error) {
	return &acp.RequestPermissionResponse{Outcome: acp.PermissionOutcome{Outcome: "allowed"}}, nil
}
