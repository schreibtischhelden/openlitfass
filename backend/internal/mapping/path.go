// Package mapping resolves dot-notation paths against generic JSON-shaped
// trees (nested map[string]interface{} / []interface{}) and turns one such
// "item" into a model.Event according to a per-source SourceMapping — the
// mechanism that lets new RSS/JSON sources be added via config instead of Go
// code (see <instance>/sources/*.json and sources/generic_json.go,
// sources/generic_rss.go).
package mapping

import (
	"fmt"
	"strconv"
	"strings"
)

// Get resolves a dot-separated path like "location.title" or "tags.0"
// against a generic tree. Returns the raw value and whether it was found.
// An empty path returns node itself.
func Get(node interface{}, path string) (interface{}, bool) {
	if path == "" {
		return node, node != nil
	}
	cur := node
	for _, p := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]interface{}:
			val, ok := v[p]
			if !ok {
				return nil, false
			}
			cur = val
		case []interface{}:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, cur != nil
}

// String coerces a resolved value to a string ("" if missing/nil).
func String(node interface{}, path string) string {
	v, ok := Get(node, path)
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

// Float coerces a resolved value to float64 (0 if missing/unparseable).
func Float(node interface{}, path string) float64 {
	v, ok := Get(node, path)
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case string:
		// German decimal comma ("19,62") as well as "19.62".
		f, _ := strconv.ParseFloat(strings.Replace(strings.TrimSpace(t), ",", ".", 1), 64)
		return f
	default:
		return 0
	}
}

// Bool coerces a resolved value to bool (false if missing/unparseable).
func Bool(node interface{}, path string) bool {
	v, ok := Get(node, path)
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(t))
		return b
	case float64:
		return t != 0
	default:
		return false
	}
}
