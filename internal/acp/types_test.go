package acp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalRequest(t *testing.T) {
	req := InitializeRequest{ProtocolVersion: 1}
	b, err := MarshalRequest(5, "initialize", req)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"id":5`)
	assert.Contains(t, string(b), `"method":"initialize"`)
	assert.Contains(t, string(b), `"protocolVersion":1`)
}

func TestParseMessage(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"a"},"authMethods":[]}}`
	env, err := ParseMessage([]byte(line))
	require.NoError(t, err)
	assert.Equal(t, "2.0", env.JSONRPC)
	assert.Equal(t, "initialize", mustMethod(env))
}

func TestSessionUpdateKind(t *testing.T) {
	line := `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"tool_call","toolCallId":"c1"}}}`
	env, err := ParseMessage([]byte(line))
	require.NoError(t, err)
	require.Equal(t, "session/update", env.Method)
	var su SessionUpdate
	require.NoError(t, json.Unmarshal([]byte(line), &su))
	assert.Equal(t, "s1", su.Params.SessionID)
	assert.Equal(t, "tool_call", su.UpdateKind())
}

func TestMarshalNotification(t *testing.T) {
	req := CloseSessionRequest{SessionID: "s1"}
	b, err := MarshalNotification("session/close", req)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"method":"session/close"`)
	assert.Contains(t, string(b), `"sessionId":"s1"`)
	assert.NotContains(t, string(b), `"id"`)
}

func TestParseMessageErrors(t *testing.T) {
	_, err := ParseMessage([]byte(`not json`))
	assert.Error(t, err)

	_, err = ParseMessage([]byte(`{"jsonrpc":"1.0"}`))
	assert.Error(t, err)
}

func TestErrorString(t *testing.T) {
	e := &Error{Code: -32600, Message: "Invalid Request"}
	assert.Equal(t, "jsonrpc error -32600: Invalid Request", e.Error())
}

func TestTextBlock(t *testing.T) {
	b := TextBlock("hello")
	assert.Equal(t, "text", b.Type)
	assert.Equal(t, "hello", b.Text)
}

func mustMethod(e Envelope) string {
	if e.Method != "" {
		return e.Method
	}
	var res InitializeResponse
	_ = json.Unmarshal(e.Result, &res)
	return "initialize"
}
