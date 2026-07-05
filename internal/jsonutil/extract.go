// Package jsonutil provides small, reusable helpers for working with JSON
// responses in cttw.
package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ExtractOutermost finds the first occurrence of open ('[' or '{') that begins
// a valid JSON value and returns that value as raw bytes. It respects JSON
// string escaping, so brackets inside strings do not confuse extraction.
func ExtractOutermost(data []byte, open byte) ([]byte, error) {
	var close byte
	switch open {
	case '[':
		close = ']'
	case '{':
		close = '}'
	default:
		return nil, fmt.Errorf("unsupported open delimiter %q", open)
	}

	start := 0
	for {
		idx := bytes.IndexByte(data[start:], open)
		if idx < 0 {
			return nil, fmt.Errorf("no JSON %c%c found", open, close)
		}
		candidate := start + idx
		dec := json.NewDecoder(bytes.NewReader(data[candidate:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err == nil {
			return raw, nil
		}
		start = candidate + 1
	}
}
