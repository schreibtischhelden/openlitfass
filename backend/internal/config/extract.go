package config

import "encoding/json"

// ExtractRule pulls one named value out of an event container of an "html"
// source. In JSON it's either a plain selector string ("h2") or an object.
type ExtractRule struct {
	// Selector is a CSS selector relative to the item (see internal/htmlsel).
	// Empty means the item itself.
	Selector string `json:"selector,omitempty"`
	// Attr reads an attribute instead of the text, e.g. "href" (relative
	// URLs are resolved against the page URL).
	Attr string `json:"attr,omitempty"`
	// HTML returns the inner HTML instead of the text - for descriptions, so
	// paragraph breaks survive (the mapping cleans the HTML afterwards).
	HTML bool `json:"html,omitempty"`
	// All joins every match (instead of only the first), skipping the first
	// Skip matches - e.g. "all paragraphs after the first three".
	All  bool `json:"all,omitempty"`
	Skip int  `json:"skip,omitempty"`
	// Regex extracts part of the value: named group "value", else the first
	// group, else the whole match; no match = empty.
	Regex string `json:"regex,omitempty"`
}

// UnmarshalJSON accepts the "selector only" shorthand: "extract": {"title": "h2"}.
func (r *ExtractRule) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*r = ExtractRule{Selector: s}
		return nil
	}
	type plain ExtractRule
	return json.Unmarshal(b, (*plain)(r))
}

// MarshalJSON writes the shorthand back when only a selector is set, so
// RecordAttempt doesn't blow hand-written source files up.
func (r ExtractRule) MarshalJSON() ([]byte, error) {
	if r == (ExtractRule{Selector: r.Selector}) {
		return json.Marshal(r.Selector)
	}
	type plain ExtractRule
	return json.Marshal(plain(r))
}
