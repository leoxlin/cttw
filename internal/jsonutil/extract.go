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

	inString := false
	escape := false
	for i := 0; i < len(data); i++ {
		b := data[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		if b == '"' {
			inString = true
			continue
		}
		if b != open {
			continue
		}

		candidate := i
		dec := json.NewDecoder(bytes.NewReader(data[candidate:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err == nil {
			return raw, nil
		}
	}

	return nil, fmt.Errorf("no JSON %c%c found", open, close)
}
