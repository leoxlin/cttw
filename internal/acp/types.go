package acp

import (
	"encoding/json"
	"fmt"
)

const (
	StopReasonEndTurn     = "end_turn"
	StopReasonCancelled   = "cancelled"
	StopReasonMaxTokens   = "max_tokens"
	StopReasonMaxRequests = "max_turn_requests"
	StopReasonRefusal     = "refusal"
)

// Envelope is a raw JSON-RPC 2.0 message.
type Envelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// MarshalRequest returns a JSON-RPC request line (without trailing newline).
func MarshalRequest(id int64, method string, params any) ([]byte, error) {
	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{
		JSONRPC: "2.0",
		ID:      json.RawMessage(fmt.Sprintf("%d", id)),
		Method:  method,
		Params:  p,
	})
}

// MarshalNotification returns a JSON-RPC notification line.
func MarshalNotification(method string, params any) ([]byte, error) {
	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{
		JSONRPC: "2.0",
		Method:  method,
		Params:  p,
	})
}

// ParseMessage parses a single JSON-RPC line.
func ParseMessage(line []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, fmt.Errorf("parse jsonrpc: %w", err)
	}
	if env.JSONRPC != "2.0" {
		return Envelope{}, fmt.Errorf("unsupported jsonrpc version: %s", env.JSONRPC)
	}
	return env, nil
}

// Info describes an implementation.
type Info struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// Capabilities.
type ClientCapabilities struct {
	FS       FSCapabilities `json:"fs"`
	Terminal bool           `json:"terminal"`
}

type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type AgentCapabilities struct {
	LoadSession        bool               `json:"loadSession,omitempty"`
	PromptCapabilities PromptCapabilities `json:"promptCapabilities,omitempty"`
	McpCapabilities    McpCapabilities    `json:"mcpCapabilities,omitempty"`
}

type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

type McpCapabilities struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Initialize.
type InitializeRequest struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         Info               `json:"clientInfo"`
}

type InitializeResponse struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AgentInfo         Info              `json:"agentInfo"`
	AuthMethods       []AuthMethod      `json:"authMethods"`
}

// Session.
type NewSessionRequest struct {
	CWD                   string      `json:"cwd"`
	AdditionalDirectories []string    `json:"additionalDirectories,omitempty"`
	McpServers            []McpServer `json:"mcpServers,omitempty"`
}

type McpServer struct {
	Name    string        `json:"name"`
	Command string        `json:"command,omitempty"`
	Args    []string      `json:"args,omitempty"`
	Env     []EnvVariable `json:"env,omitempty"`
	URL     string        `json:"url,omitempty"`
}

type EnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type NewSessionResponse struct {
	SessionID string `json:"sessionId"`
}

type CloseSessionRequest struct {
	SessionID string `json:"sessionId"`
}

// Prompt.
type PromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type PromptResponse struct {
	StopReason string `json:"stopReason"`
}

// ContentBlock is a simplified union; only text is required for cttw.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// Session update notification.
type SessionUpdate struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	} `json:"params"`
}

func (s SessionUpdate) UpdateKind() string {
	var aux struct {
		SessionUpdate string `json:"sessionUpdate"`
	}
	_ = json.Unmarshal(s.Params.Update, &aux)
	return aux.SessionUpdate
}

// Client methods: fs.
type ReadTextFileRequest struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type ReadTextFileResponse struct {
	Content string `json:"content"`
}

type WriteTextFileRequest struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

// Client methods: terminal.
type CreateTerminalRequest struct {
	SessionID       string        `json:"sessionId"`
	Command         string        `json:"command"`
	Args            []string      `json:"args,omitempty"`
	Env             []EnvVariable `json:"env,omitempty"`
	CWD             string        `json:"cwd,omitempty"`
	OutputByteLimit int           `json:"outputByteLimit,omitempty"`
}

type CreateTerminalResponse struct {
	TerminalID string `json:"terminalId"`
}

type TerminalOutputRequest struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

type TerminalOutputResponse struct {
	Output     string              `json:"output"`
	Truncated  bool                `json:"truncated"`
	ExitStatus *TerminalExitStatus `json:"exitStatus,omitempty"`
}

type TerminalExitStatus struct {
	ExitCode int    `json:"exitCode"`
	Signal   string `json:"signal,omitempty"`
}

type WaitForTerminalExitRequest struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

type WaitForTerminalExitResponse struct {
	ExitCode int    `json:"exitCode"`
	Signal   string `json:"signal,omitempty"`
}

type ReleaseTerminalRequest struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// Client methods: permission.
type RequestPermissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  PermissionToolCall `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

type PermissionToolCall struct {
	ToolCallID string `json:"toolCallId"`
	Title      string `json:"title,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

type RequestPermissionResponse struct {
	Outcome PermissionOutcome `json:"outcome"`
}

type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}
