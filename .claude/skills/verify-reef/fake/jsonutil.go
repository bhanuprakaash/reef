package main

import (
	"bytes"
	"encoding/json"
)

// marshalLine is v as one line of compact JSON, HTML characters left unescaped (recipes
// grep raw text like "[channel" out of these lines).
func marshalLine(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimRight(buf.Bytes(), "\n")
}
