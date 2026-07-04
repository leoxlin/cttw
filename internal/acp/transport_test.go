package acp

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdioTransport_SendRecv(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()

	client := NewStdioTransport(inWriter, outReader)
	go func() {
		_ = client.Start(context.Background())
	}()

	server := NewStdioTransport(outWriter, inReader)
	go func() {
		_ = server.Start(context.Background())
	}()

	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	require.NoError(t, client.Send(context.Background(), msg))

	got, err := server.Recv(context.Background())
	require.NoError(t, err)
	assert.Equal(t, msg, got)

	_ = client.Close()
	_ = server.Close()
}
