package structuredio

import (
	"bytes"
	"encoding/json"
)

// NDJSONSafeMarshal serializes value as one compact JSON record for NDJSON
// transports. U+2028 and U+2029 are escaped because some JavaScript receivers
// treat them as line terminators while splitting streams.
func NDJSONSafeMarshal(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}

	out := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
	out = bytes.ReplaceAll(out, []byte("\u2028"), []byte("\\u2028"))
	out = bytes.ReplaceAll(out, []byte("\u2029"), []byte("\\u2029"))
	return out, nil
}
