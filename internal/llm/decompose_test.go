package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClient struct {
	resp string
}

func (f *fakeClient) Chat(ctx context.Context, system, user string) (string, error) {
	return f.resp, nil
}

func TestDecomposeTask(t *testing.T) {
	resp := `[{"title":"add model","description":"define User struct"},{"title":"add handler","description":"add HTTP handler","depends_on":1}]`
	chunks, err := DecomposeTask(context.Background(), &fakeClient{resp: resp}, "build user API")
	require.NoError(t, err)
	require.Len(t, chunks, 2)
	assert.Equal(t, "add model", chunks[0].Title)
	assert.Equal(t, "add handler", chunks[1].Title)
}
