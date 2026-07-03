package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Chat(t *testing.T) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "key", "gpt-4o", ts.Client())
	resp, err := c.Chat(context.Background(), "sys", "user")
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "hello", resp)
}
