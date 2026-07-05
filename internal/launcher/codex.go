package launcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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

	// Reap the child process to avoid zombies and log unexpected exits.
	go func() {
		if err := cmd.Wait(); err != nil {
			// Only log if the process exited with an error. Context cancellation
			// may also surface here; log at a low level for observability.
			fmt.Fprintf(os.Stderr, "codex agent exited: %v\n", err)
		}
	}()

	transport := acp.NewStdioTransport(stdin, stdout)
	client := acp.NewClient(transport)
	client.SetHandler(&defaultHandler{cwd: spec.Repo.LocalDir})

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
	client    *acp.Client
	cmd       *exec.Cmd
	spec      LaunchSpec
	sessionID string
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
	res, err := a.client.NewSession(ctx, req)
	if err != nil {
		return err
	}
	if res.SessionID != "" {
		a.sessionID = res.SessionID
	}
	return nil
}

func (a *codexAgent) SessionID() string {
	return a.sessionID
}

func (a *codexAgent) Prompt(ctx context.Context, prompt string) (*acp.PromptResponse, error) {
	return a.client.Prompt(ctx, acp.PromptRequest{
		SessionID: a.sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
}

func (a *codexAgent) Close(ctx context.Context) error {
	if a.sessionID != "" {
		_ = a.client.CloseSession(ctx, acp.CloseSessionRequest{SessionID: a.sessionID})
	}
	return a.client.Close()
}

// defaultHandler provides real implementations for fs/terminal client methods.
type defaultHandler struct {
	cwd       string
	mu        sync.Mutex
	terminals map[string]*terminalSession
}

type terminalSession struct {
	cmd    *exec.Cmd
	stdout *strings.Builder
	stderr *strings.Builder
	done   chan struct{}
	exit   *acp.TerminalExitStatus
}

func (d *defaultHandler) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if d.cwd != "" {
		return filepath.Join(d.cwd, path)
	}
	return path
}

func (d *defaultHandler) HandleReadTextFile(ctx context.Context, req acp.ReadTextFileRequest) (*acp.ReadTextFileResponse, error) {
	path := d.resolvePath(req.Path)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var content strings.Builder
	scanner := bufio.NewScanner(f)
	line := 1
	linesRead := 0
	for scanner.Scan() {
		if req.Line > 0 && line < req.Line {
			line++
			continue
		}
		if req.Limit > 0 && linesRead >= req.Limit {
			break
		}
		content.WriteString(scanner.Text())
		content.WriteByte('\n')
		line++
		linesRead++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &acp.ReadTextFileResponse{Content: content.String()}, nil
}

func (d *defaultHandler) HandleWriteTextFile(ctx context.Context, req acp.WriteTextFileRequest) error {
	path := d.resolvePath(req.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(req.Content), 0644)
}

func (d *defaultHandler) HandleCreateTerminal(ctx context.Context, req acp.CreateTerminalRequest) (*acp.CreateTerminalResponse, error) {
	cwd := req.CWD
	if cwd == "" {
		cwd = d.cwd
	}
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	cmd.Dir = cwd
	if len(req.Env) > 0 {
		env := os.Environ()
		for _, e := range req.Env {
			env = append(env, e.Name+"="+e.Value)
		}
		cmd.Env = env
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	session := &terminalSession{
		cmd:    cmd,
		stdout: &stdout,
		stderr: &stderr,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(session.done)
		err := cmd.Wait()
		exit := &acp.TerminalExitStatus{ExitCode: 0}
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exit.ExitCode = exitErr.ExitCode()
			} else {
				exit.ExitCode = -1
			}
		}
		session.exit = exit
	}()

	id := strconv.Itoa(int(time.Now().UnixNano()))
	d.mu.Lock()
	if d.terminals == nil {
		d.terminals = make(map[string]*terminalSession)
	}
	d.terminals[id] = session
	d.mu.Unlock()

	return &acp.CreateTerminalResponse{TerminalID: id}, nil
}

func (d *defaultHandler) HandleTerminalOutput(ctx context.Context, req acp.TerminalOutputRequest) (*acp.TerminalOutputResponse, error) {
	d.mu.Lock()
	session, ok := d.terminals[req.TerminalID]
	d.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("terminal %q not found", req.TerminalID)
	}

	out := session.stdout.String() + session.stderr.String()
	truncated := false
	const limit = 1024 * 1024 // 1 MiB default
	if len(out) > limit {
		out = out[:limit]
		truncated = true
	}

	resp := &acp.TerminalOutputResponse{Output: out, Truncated: truncated}
	if session.exit != nil {
		resp.ExitStatus = session.exit
	}
	return resp, nil
}

func (d *defaultHandler) HandleWaitForTerminalExit(ctx context.Context, req acp.WaitForTerminalExitRequest) (*acp.WaitForTerminalExitResponse, error) {
	d.mu.Lock()
	session, ok := d.terminals[req.TerminalID]
	d.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("terminal %q not found", req.TerminalID)
	}

	select {
	case <-session.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	exit := &acp.WaitForTerminalExitResponse{ExitCode: 0}
	if session.exit != nil {
		exit.ExitCode = session.exit.ExitCode
		exit.Signal = session.exit.Signal
	}
	return exit, nil
}

func (d *defaultHandler) HandleReleaseTerminal(ctx context.Context, req acp.ReleaseTerminalRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if session, ok := d.terminals[req.TerminalID]; ok {
		if session.cmd != nil && session.cmd.Process != nil {
			_ = session.cmd.Process.Kill()
		}
		delete(d.terminals, req.TerminalID)
	}
	return nil
}

func (d *defaultHandler) HandleRequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (*acp.RequestPermissionResponse, error) {
	return &acp.RequestPermissionResponse{Outcome: acp.PermissionOutcome{Outcome: "allowed"}}, nil
}

func (d *defaultHandler) HandleNotification(ctx context.Context, method string, params json.RawMessage) error {
	// Notifications are best-effort; ignore by default.
	return nil
}
