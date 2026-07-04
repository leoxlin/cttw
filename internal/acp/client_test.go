package acp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHandler records calls and returns canned responses.
type mockHandler struct {
	readFile ReadTextFileResponse
}

func (m *mockHandler) HandleReadTextFile(ctx context.Context, req ReadTextFileRequest) (*ReadTextFileResponse, error) {
	return &m.readFile, nil
}

func (m *mockHandler) HandleWriteTextFile(ctx context.Context, req WriteTextFileRequest) error {
	return nil
}

func (m *mockHandler) HandleCreateTerminal(ctx context.Context, req CreateTerminalRequest) (*CreateTerminalResponse, error) {
	return &CreateTerminalResponse{TerminalID: "t1"}, nil
}

func (m *mockHandler) HandleTerminalOutput(ctx context.Context, req TerminalOutputRequest) (*TerminalOutputResponse, error) {
	return &TerminalOutputResponse{Output: "out"}, nil
}

func (m *mockHandler) HandleWaitForTerminalExit(ctx context.Context, req WaitForTerminalExitRequest) (*WaitForTerminalExitResponse, error) {
	return &WaitForTerminalExitResponse{ExitCode: 0}, nil
}

func (m *mockHandler) HandleReleaseTerminal(ctx context.Context, req ReleaseTerminalRequest) error {
	return nil
}

func (m *mockHandler) HandleRequestPermission(ctx context.Context, req RequestPermissionRequest) (*RequestPermissionResponse, error) {
	return &RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: "allowed"}}, nil
}

func TestClient_InitializeAndPrompt(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()

	agentIn := NewStdioTransport(inWriter, outReader)
	client := NewClient(NewStdioTransport(outWriter, inReader))
	client.SetHandler(&mockHandler{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = client.Start(ctx) }()
	go func() { _ = agentIn.Start(ctx) }()

	// Agent side: reply to initialize with a canned response.
	go func() {
		line, _ := agentIn.Recv(ctx)
		var env Envelope
		_ = json.Unmarshal(line, &env)
		assert.Equal(t, "initialize", env.Method)

		res, _ := json.Marshal(Envelope{
			JSONRPC: "2.0",
			ID:      env.ID,
			Result: json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"fake","version":"1"},"authMethods":[]}`),
		})
		_ = agentIn.Send(ctx, res)

		// Reply to prompt with stop_reason end_turn.
		line, _ = agentIn.Recv(ctx)
		_ = json.Unmarshal(line, &env)
		assert.Equal(t, "prompt", env.Method)
		res, _ = json.Marshal(Envelope{
			JSONRPC: "2.0",
			ID:      env.ID,
			Result:  json.RawMessage(`{"stopReason":"end_turn"}`),
		})
		_ = agentIn.Send(ctx, res)
	}()

	initRes, err := client.Initialize(ctx, InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo:      Info{Name: "cttw", Version: "0.1"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, initRes.ProtocolVersion)

	promptRes, err := client.Prompt(ctx, PromptRequest{
		SessionID: "s1",
		Prompt:    []ContentBlock{TextBlock("hello")},
	})
	require.NoError(t, err)
	assert.Equal(t, StopReasonEndTurn, promptRes.StopReason)

	_ = client.Close()
	_ = agentIn.Close()
}

func TestClient_HandlerRespondsToServerRequest(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()

	agentIn := NewStdioTransport(inWriter, outReader)
	client := NewClient(NewStdioTransport(outWriter, inReader))
	client.SetHandler(&mockHandler{readFile: ReadTextFileResponse{Content: "file contents"}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = client.Start(ctx) }()
	go func() { _ = agentIn.Start(ctx) }()

	// Agent sends a readTextFile request.
	req, _ := MarshalRequest(1, "fs/readTextFile", ReadTextFileRequest{SessionID: "s1", Path: "/foo.txt"})
	require.NoError(t, agentIn.Send(ctx, req))

	// Read the response from the agent side.
	var got Envelope
	require.Eventually(t, func() bool {
		line, err := agentIn.Recv(ctx)
		if err != nil {
			return false
		}
		_ = json.Unmarshal(line, &got)
		return got.ID != nil
	}, time.Second, 10*time.Millisecond)

	require.NotNil(t, got.Result)
	var res ReadTextFileResponse
	require.NoError(t, json.Unmarshal(got.Result, &res))
	assert.Equal(t, "file contents", res.Content)

	_ = client.Close()
	_ = agentIn.Close()
}
