package jsonutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractOutermost(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		open    byte
		want    string
		wantErr bool
	}{
		{
			name:  "array",
			input: `[{"title":"t1","description":"d1"}]`,
			open:  '[',
			want:  `[{"title":"t1","description":"d1"}]`,
		},
		{
			name:  "object",
			input: `{"status":"completed","pr_number":42}`,
			open:  '{',
			want:  `{"status":"completed","pr_number":42}`,
		},
		{
			name:  "array embedded in text",
			input: "Here is the result:\n[{\"title\":\"t1\",\"description\":\"d1\"}]\nDone.",
			open:  '[',
			want:  `[{"title":"t1","description":"d1"}]`,
		},
		{
			name:  "object embedded in text",
			input: "done\n{\"status\":\"completed\",\"pr_number\":42}\nok",
			open:  '{',
			want:  `{"status":"completed","pr_number":42}`,
		},
		{
			name:  "brackets inside strings are ignored",
			input: `[{"title":"a]b","description":"d1"}]`,
			open:  '[',
			want:  `[{"title":"a]b","description":"d1"}]`,
		},
		{
			name:  "braces inside strings are ignored",
			input: `{"status":"{completed}","pr_number":42}`,
			open:  '{',
			want:  `{"status":"{completed}","pr_number":42}`,
		},
		{
			name:    "no array",
			input:   "no json here",
			open:    '[',
			wantErr: true,
		},
		{
			name:    "no object",
			input:   "no json here",
			open:    '{',
			wantErr: true,
		},
		{
			name:  "json inside string literal before real array",
			input: `"[{"title":"fake"}]" [{"title":"real"}]`,
			open:  '[',
			want:  `[{"title":"real"}]`,
		},
		{
			name:  "json inside string literal before real object",
			input: `"{"status":"fake"}" {"status":"real"}`,
			open:  '{',
			want:  `{"status":"real"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractOutermost([]byte(tt.input), tt.open)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}
