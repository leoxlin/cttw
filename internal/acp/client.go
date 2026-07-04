package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Handler receives agent-to-client method calls and returns responses.
type Handler interface {
	HandleReadTextFile(ctx context.Context, req ReadTextFileRequest) (*ReadTextFileResponse, error)
	HandleWriteTextFile(ctx context.Context, req WriteTextFileRequest) error
	HandleCreateTerminal(ctx context.Context, req CreateTerminalRequest) (*CreateTerminalResponse, error)
	HandleTerminalOutput(ctx context.Context, req TerminalOutputRequest) (*TerminalOutputResponse, error)
	HandleWaitForTerminalExit(ctx context.Context, req WaitForTerminalExitRequest) (*WaitForTerminalExitResponse, error)
	HandleReleaseTerminal(ctx context.Context, req ReleaseTerminalRequest) error
	HandleRequestPermission(ctx context.Context, req RequestPermissionRequest) (*RequestPermissionResponse, error)
}

// Client is an ACP JSON-RPC client over a Transport.
type Client struct {
	transport Transport
	handler   Handler

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan Envelope
	closed  bool
	closeCh chan struct{}
	// errCh is a best-effort sink for malformed messages and unmatched responses.
	errCh   chan error
}

func NewClient(transport Transport) *Client {
	return &Client{
		transport: transport,
		pending:   make(map[int64]chan Envelope),
		closeCh:   make(chan struct{}),
		errCh:     make(chan error, 1),
	}
}

func (c *Client) SetHandler(h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = h
}

// Start begins reading messages from the transport and dispatching them.
func (c *Client) Start(ctx context.Context) error {
	// Drive the transport read loop so Recv can consume buffered lines.
	go func() {
		_ = c.transport.Start(ctx)
	}()

	for {
		line, err := c.transport.Recv(ctx)
		if err != nil {
			if err == io.EOF || c.isClosed() {
				return nil
			}
			return err
		}
		env, err := ParseMessage(line)
		if err != nil {
			select {
			case c.errCh <- fmt.Errorf("acp: malformed message: %w", err):
			default:
			}
			continue
		}
		if env.ID != nil && env.Method != "" {
			// Agent request; dispatch to handler and respond.
			if err := c.handleRequest(ctx, env); err != nil {
				// Log or surface? For now best-effort response.
				_ = c.sendError(ctx, env.ID, -32000, err.Error())
			}
			continue
		}
		if env.ID != nil {
			c.routeResponse(env)
		}
	}
}

func (c *Client) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *Client) routeResponse(env Envelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var id int64
	if err := json.Unmarshal(env.ID, &id); err != nil {
		select {
		case c.errCh <- fmt.Errorf("acp: malformed response id: %w", err):
		default:
		}
		return
	}
	ch, ok := c.pending[id]
	if !ok {
		select {
		case c.errCh <- fmt.Errorf("acp: unmatched response id %d", id):
		default:
		}
		return
	}
	select {
	case ch <- env:
	default:
	}
}

func (c *Client) handleRequest(ctx context.Context, env Envelope) error {
	c.mu.Lock()
	h := c.handler
	c.mu.Unlock()
	if h == nil {
		return c.sendError(ctx, env.ID, -32601, "no handler")
	}

	switch env.Method {
	case "fs/readTextFile":
		var req ReadTextFileRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return c.sendError(ctx, env.ID, -32700, err.Error())
		}
		res, err := h.HandleReadTextFile(ctx, req)
		return c.sendResult(ctx, env.ID, res, err)
	case "fs/writeTextFile":
		var req WriteTextFileRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return c.sendError(ctx, env.ID, -32700, err.Error())
		}
		err := h.HandleWriteTextFile(ctx, req)
		return c.sendResult(ctx, env.ID, struct{}{}, err)
	case "terminal/createTerminal":
		var req CreateTerminalRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return c.sendError(ctx, env.ID, -32700, err.Error())
		}
		res, err := h.HandleCreateTerminal(ctx, req)
		return c.sendResult(ctx, env.ID, res, err)
	case "terminal/output":
		var req TerminalOutputRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return c.sendError(ctx, env.ID, -32700, err.Error())
		}
		res, err := h.HandleTerminalOutput(ctx, req)
		return c.sendResult(ctx, env.ID, res, err)
	case "terminal/waitForExit":
		var req WaitForTerminalExitRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return c.sendError(ctx, env.ID, -32700, err.Error())
		}
		res, err := h.HandleWaitForTerminalExit(ctx, req)
		return c.sendResult(ctx, env.ID, res, err)
	case "terminal/release":
		var req ReleaseTerminalRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return c.sendError(ctx, env.ID, -32700, err.Error())
		}
		err := h.HandleReleaseTerminal(ctx, req)
		return c.sendResult(ctx, env.ID, struct{}{}, err)
	case "permission/request":
		var req RequestPermissionRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return c.sendError(ctx, env.ID, -32700, err.Error())
		}
		res, err := h.HandleRequestPermission(ctx, req)
		return c.sendResult(ctx, env.ID, res, err)
	default:
		return c.sendError(ctx, env.ID, -32601, fmt.Sprintf("method not found: %s", env.Method))
	}
}

func (c *Client) call(ctx context.Context, method string, params any) (Envelope, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	line, err := MarshalRequest(id, method, params)
	if err != nil {
		return Envelope{}, err
	}

	ch := make(chan Envelope, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return Envelope{}, fmt.Errorf("client closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.transport.Send(ctx, line); err != nil {
		return Envelope{}, err
	}

	select {
	case env, ok := <-ch:
		if !ok {
			return Envelope{}, fmt.Errorf("client closed")
		}
		return env, nil
	case <-ctx.Done():
		return Envelope{}, ctx.Err()
	case <-c.closeCh:
		return Envelope{}, fmt.Errorf("client closed")
	}
}

func (c *Client) sendResult(ctx context.Context, id json.RawMessage, result any, err error) error {
	if err != nil {
		return c.sendError(ctx, id, -32000, err.Error())
	}
	b, err := json.Marshal(result)
	if err != nil {
		return c.sendError(ctx, id, -32603, err.Error())
	}
	res, err := json.Marshal(Envelope{JSONRPC: "2.0", ID: id, Result: b})
	if err != nil {
		return err
	}
	return c.transport.Send(ctx, res)
}

func (c *Client) sendError(ctx context.Context, id json.RawMessage, code int, message string) error {
	res, err := json.Marshal(Envelope{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: message},
	})
	if err != nil {
		return err
	}
	return c.transport.Send(ctx, res)
}

func (c *Client) Initialize(ctx context.Context, req InitializeRequest) (*InitializeResponse, error) {
	env, err := c.call(ctx, "initialize", req)
	if err != nil {
		return nil, err
	}
	if env.Error != nil {
		return nil, env.Error
	}
	var res InitializeResponse
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) NewSession(ctx context.Context, req NewSessionRequest) (*NewSessionResponse, error) {
	env, err := c.call(ctx, "newSession", req)
	if err != nil {
		return nil, err
	}
	if env.Error != nil {
		return nil, env.Error
	}
	var res NewSessionResponse
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) CloseSession(ctx context.Context, req CloseSessionRequest) error {
	env, err := c.call(ctx, "closeSession", req)
	if err != nil {
		return err
	}
	if env.Error != nil {
		return env.Error
	}
	return nil
}

func (c *Client) Prompt(ctx context.Context, req PromptRequest) (*PromptResponse, error) {
	env, err := c.call(ctx, "prompt", req)
	if err != nil {
		return nil, err
	}
	if env.Error != nil {
		return nil, env.Error
	}
	var res PromptResponse
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.closeCh)
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = nil
	c.mu.Unlock()
	return c.transport.Close()
}
