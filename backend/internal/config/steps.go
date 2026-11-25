package config

// Step is one request a source makes before its main request (see
// SourceConfig.Steps). It collects named lists of values instead of
// events; later steps and the main request use them through {name}
// placeholders in their URL or through SourceConfig.ForEach.
type Step struct {
	// Type is "html" (default) or "json" and decides how Collect reads the
	// response.
	Type string `json:"type,omitempty"`
	// URL may contain {today}, {until} and {name} for variables collected
	// by earlier steps (their first value).
	URL string `json:"url"`
	// Collect names the variables this step produces. A variable that ends
	// up empty is an error, so a changed page shows up in the source status.
	Collect map[string]CollectRule `json:"collect"`
}

// CollectRule reads one variable out of a step's response. Every match is
// collected; values are trimmed, empty ones dropped and duplicates removed
// (first occurrence wins).
type CollectRule struct {
	// Selector (html steps) is a CSS selector matched against the whole page.
	Selector string `json:"selector,omitempty"`
	// Attr (html steps) reads an attribute instead of the text; relative
	// href/src values are resolved against the step URL.
	Attr string `json:"attr,omitempty"`
	// Path (json steps) is a dot-path; a "*" segment stands for every
	// element of an array, e.g. "data.*.id".
	Path string `json:"path,omitempty"`
	// Regex narrows each value like ExtractRule.Regex; no match drops it.
	Regex string `json:"regex,omitempty"`
	// Split splits each value further, e.g. "," for "a,b,c".
	Split string `json:"split,omitempty"`
}

// ForEach turns the main request into several: one per value of the
// collected variable Var, or, with Param, Batch values per request added
// as repeated query parameters (Param=v1&Param=v2...).
type ForEach struct {
	Var string `json:"var"`
	// Param is the query parameter name, e.g. "object[]". Without it the
	// main URL must contain {item}, which is replaced by each value (the
	// value as it is when {item} is the whole URL, query-escaped otherwise).
	Param string `json:"param,omitempty"`
	// Batch is the number of values per request when Param is set
	// (default 1).
	Batch int `json:"batch,omitempty"`
}
