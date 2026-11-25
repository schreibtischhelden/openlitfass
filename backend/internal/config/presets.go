package config

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

// presetFS holds the source presets shipped with the core, one file per
// preset (the file name without .json is the preset name).
//
//go:embed presets/*.json
var presetFS embed.FS

// Preset is a source config template for a platform that many sites use
// with the same markup, e.g. an event widget. A source file refers to it by
// name and only supplies the parameters (see SourceConfig.Preset).
type Preset struct {
	Name        string                 `json:"-"`
	Label       string                 `json:"label"`
	Description string                 `json:"description"`
	Params      map[string]PresetParam `json:"params"`
	// Source is a SourceConfig in which every string may contain
	// {param} placeholders for the declared Params.
	Source json.RawMessage `json:"source"`
}

// PresetParam describes one parameter of a preset.
type PresetParam struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
	Example     string `json:"example,omitempty"`
}

// Presets returns every shipped preset, sorted by name.
func Presets() ([]Preset, error) {
	entries, err := presetFS.ReadDir("presets")
	if err != nil {
		return nil, err
	}
	out := make([]Preset, 0, len(entries))
	for _, e := range entries {
		p, err := loadPreset(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func loadPreset(name string) (Preset, error) {
	var p Preset
	if name == "" || strings.ContainsAny(name, `/\.`) {
		return p, fmt.Errorf("unknown preset %q", name)
	}
	data, err := presetFS.ReadFile(path.Join("presets", name+".json"))
	if err != nil {
		return p, fmt.Errorf("unknown preset %q", name)
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("preset %q: %w", name, err)
	}
	p.Name = name
	return p, nil
}

// parseSource decodes one source file. With "preset" set, the preset's
// source config (placeholders filled from "params") is decoded first and
// the file decoded on top of it: plain values and arrays in the file
// replace the preset's, objects such as "mapping" or "extract" are merged
// key by key (encoding/json's behaviour when decoding into a filled value).
func parseSource(data []byte) (SourceConfig, error) {
	var head struct {
		Preset string            `json:"preset"`
		Params map[string]string `json:"params"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return SourceConfig{}, fmt.Errorf("invalid json: %w", err)
	}

	var c SourceConfig
	if head.Preset != "" {
		p, err := loadPreset(head.Preset)
		if err != nil {
			return c, err
		}
		base, err := p.render(head.Params)
		if err != nil {
			return c, err
		}
		if err := json.Unmarshal(base, &c); err != nil {
			return c, fmt.Errorf("preset %q: %w", p.Name, err)
		}
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("invalid json: %w", err)
	}
	return c, nil
}

// render returns the preset's source config with {param} replaced in
// every string. Missing required parameters and parameters the preset
// doesn't declare are errors, so a typo doesn't fail silently.
func (p *Preset) render(given map[string]string) ([]byte, error) {
	for k := range given {
		if _, ok := p.Params[k]; !ok {
			return nil, fmt.Errorf("preset %q has no parameter %q", p.Name, k)
		}
	}
	values := make(map[string]string, len(p.Params))
	var missing []string
	for k, def := range p.Params {
		v := strings.TrimSpace(given[k])
		if v == "" {
			v = def.Default
		}
		if v == "" && def.Required {
			missing = append(missing, k)
		}
		values["{"+k+"}"] = v
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("preset %q: missing parameter(s) %s", p.Name, strings.Join(missing, ", "))
	}

	var tree interface{}
	if err := json.Unmarshal(p.Source, &tree); err != nil {
		return nil, fmt.Errorf("preset %q: %w", p.Name, err)
	}
	return json.Marshal(fillPlaceholders(tree, values))
}

func fillPlaceholders(node interface{}, values map[string]string) interface{} {
	switch v := node.(type) {
	case string:
		for ph, val := range values {
			v = strings.ReplaceAll(v, ph, val)
		}
		return v
	case map[string]interface{}:
		for k, child := range v {
			v[k] = fillPlaceholders(child, values)
		}
		return v
	case []interface{}:
		for i, child := range v {
			v[i] = fillPlaceholders(child, values)
		}
		return v
	}
	return node
}
