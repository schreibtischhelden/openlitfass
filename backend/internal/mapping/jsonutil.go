package mapping

import (
	"bytes"
	"encoding/json"
)

// MarshalNoEscape is json.Marshal without HTML escaping, for values that
// end up in hand-written source files: CSS selectors ("div > p") and
// regexes ("<b>") must stay readable instead of becoming \u003e / \u003c.
// Custom MarshalJSON methods must use it too, since json.Marshal's
// escaping is applied to their output separately.
func MarshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
