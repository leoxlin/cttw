package acp

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHandler returns canned responses for agent-to-client method calls.
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

func (m *mockHandler) HandleNotification(ctx context.Context, method string, params json.RawMessage) error {
	return nil
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
			Result:  json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"fake","version":"1"},"authMethods":[]}`),
		})
		_ = agentIn.Send(ctx, res)

		// Reply to prompt with stop_reason end_turn.
		line, _ = agentIn.Recv(ctx)
		_ = json.Unmarshal(line, &env)
		assert.Equal(t, "session/prompt", env.Method)
		update, _ := json.Marshal(Envelope{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params:  json.RawMessage(`{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"done"}}}`),
		})
		_ = agentIn.Send(ctx, update)
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
	assert.Equal(t, "done", promptRes.Content)

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

func TestClient_CloseWhilePending(t *testing.T) {
	inReader, _ := io.Pipe()
	outReader, outWriter := io.Pipe()

	client := NewClient(NewStdioTransport(outWriter, inReader))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = client.Start(ctx) }()
	// Drain the client's writes so the pending call makes it past Send.
	go func() { _, _ = io.Copy(io.Discard, outReader) }()

	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err = client.Initialize(ctx, InitializeRequest{
			ProtocolVersion: 1,
			ClientInfo:      Info{Name: "cttw", Version: "0.1"},
		})
	}()

	// Give the call time to enter the pending map before closing.
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, client.Close())
	<-done

	require.Error(t, err)
	assert.Contains(t, err.Error(), "client closed")
}

func TestClient_MalformedAndUnmatchedResponsesIgnored(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()

	agentIn := NewStdioTransport(inWriter, outReader)
	client := NewClient(NewStdioTransport(outWriter, inReader))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = client.Start(ctx) }()
	go func() { _ = agentIn.Start(ctx) }()

	go func() {
		line, _ := agentIn.Recv(ctx)
		var env Envelope
		_ = json.Unmarshal(line, &env)
		require.Equal(t, "initialize", env.Method)

		// Malformed line should not stop the read loop.
		_ = agentIn.Send(ctx, []byte("not valid json"))

		// Unmatched response should be ignored, not fatal.
		bad, _ := json.Marshal(Envelope{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`999`),
			Result:  json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"fake","version":"1"},"authMethods":[]}`),
		})
		_ = agentIn.Send(ctx, bad)

		// Correct response should still be routed to the pending call.
		good, _ := json.Marshal(Envelope{
			JSONRPC: "2.0",
			ID:      env.ID,
			Result:  json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"fake","version":"1"},"authMethods":[]}`),
		})
		_ = agentIn.Send(ctx, good)
	}()

	initRes, err := client.Initialize(ctx, InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo:      Info{Name: "cttw", Version: "0.1"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, initRes.ProtocolVersion)

	_ = client.Close()
	_ = agentIn.Close()
}

// closeInjectingTransport delivers a queued response from its Close method so
// we can exercise a response arriving while the client is shutting down.
type closeInjectingTransport struct {
	mu      sync.Mutex
	closed  bool
	lines   chan []byte
	onClose func()
}

func newCloseInjectingTransport(onClose func()) *closeInjectingTransport {
	return &closeInjectingTransport{
		lines:   make(chan []byte, 1),
		onClose: onClose,
	}
}

func (t *closeInjectingTransport) Start(ctx context.Context) error { return nil }

func (t *closeInjectingTransport) Send(ctx context.Context, data []byte) error { return nil }

func (t *closeInjectingTransport) Recv(ctx context.Context) ([]byte, error) {
	select {
	case line, ok := <-t.lines:
		if !ok {
			return nil, io.EOF
		}
		return line, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *closeInjectingTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	if t.onClose != nil {
		t.onClose()
	}
	close(t.lines)
	return nil
}

func TestClient_ResponseDuringCloseDoesNotPanic(t *testing.T) {
	transport := newCloseInjectingTransport(nil)
	client := NewClient(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.Start(ctx)
	}()

	// Queue an Initialize call so there is a pending channel in the map.
	callDone := make(chan struct{})
	var callErr error
	go func() {
		defer close(callDone)
		_, callErr = client.Initialize(ctx, InitializeRequest{
			ProtocolVersion: 1,
			ClientInfo:      Info{Name: "cttw", Version: "0.1"},
		})
	}()

	// Give the goroutine time to register the pending call.
	time.Sleep(20 * time.Millisecond)

	// Close the transport; its Close implementation injects a response that
	// would be routed after closeCh is already closed. Without the closeCh
	// select this could send on a closed pending channel and panic.
	res, _ := json.Marshal(Envelope{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Result:  json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"fake","version":"1"},"authMethods":[]}`),
	})
	transport.onClose = func() {
		transport.lines <- res
	}
	require.NotPanics(t, func() { _ = client.Close() })

	<-callDone
	require.Error(t, callErr)
	assert.Contains(t, callErr.Error(), "client closed")
	<-done
}

// eofTransport returns EOF immediately so the client read loop exits cleanly.
type eofTransport struct{}

func (eofTransport) Start(ctx context.Context) error             { return nil }
func (eofTransport) Send(ctx context.Context, data []byte) error { return nil }
func (eofTransport) Recv(ctx context.Context) ([]byte, error)    { return nil, io.EOF }
func (eofTransport) Close() error                                { return nil }

func TestClient_ConcurrentRouteResponseAndClose(t *testing.T) {
	for i := 0; i < 200; i++ {
		client := NewClient(eofTransport{})
		ctx, cancel := context.WithCancel(context.Background())
		go func() { _ = client.Start(ctx) }()

		ch := make(chan Envelope, 1)
		client.mu.Lock()
		client.pending[1] = ch
		client.mu.Unlock()

		env := Envelope{ID: json.RawMessage(`1`), Result: json.RawMessage(`{}`)}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			client.routeResponse(env)
		}()
		go func() {
			defer wg.Done()
			_ = client.Close()
		}()
		wg.Wait()

		cancel()
		_ = client.Close()
	}
}
