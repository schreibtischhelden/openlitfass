package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const infomaxSource = `{
  "label": "Veranstaltungen Stralsund",
  "preset": "infomax",
  "params": { "host": "tmvwhl.infomaxnet.de", "permalink": "stadt-stralsund-de", "widgetToken": "4YRcdHbzey0." },
  "daysAhead": 30,
  "extract": { "title": "h2" },
  "mapping": { "category": { "default": "Kultur" } }
}`

func TestParseSourceAppliesPreset(t *testing.T) {
	// Act
	c, err := parseSource([]byte(infomaxSource))

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if c.Type != "html" || c.Label != "Veranstaltungen Stralsund" {
		t.Errorf("type/label = %q/%q", c.Type, c.Label)
	}
	if want := "https://tmvwhl.infomaxnet.de/stadt-stralsund-de/de/action/items?widgetToken=4YRcdHbzey0.&outputType=itemsPage&layout=listitem&showDate=1"; c.URL != want {
		t.Errorf("url = %s, want %s (language default filled in)", c.URL, want)
	}
	if len(c.Steps) != 1 || !strings.Contains(c.Steps[0].URL, "dateFrom={today}") {
		t.Errorf("steps = %+v, want the preset step with {today} kept for the step runner", c.Steps)
	}
	if c.DaysAhead != 30 || c.TimeoutSeconds != 180 {
		t.Errorf("daysAhead/timeoutSeconds = %d/%d, want the file's 30 and the preset's 180", c.DaysAhead, c.TimeoutSeconds)
	}
	if got := c.Extract["title"].Selector; got != "h2" {
		t.Errorf("extract.title = %q, want the file's override", got)
	}
	if got := c.Extract["link"].Attr; got != "href" {
		t.Errorf("extract.link.attr = %q, want the preset's rule kept", got)
	}
	if c.Mapping == nil || c.Mapping.Category.Default != "Kultur" || c.Mapping.Title.Path != "title" {
		t.Errorf("mapping = %+v, want category overridden and title kept", c.Mapping)
	}
	if c.RequestDelaySeconds != 5 || c.CategorySeparator != ", " || !c.CategoryAttributes["Kinder"].FamilyFriendly {
		t.Errorf("requestDelaySeconds/categorySeparator/categoryAttributes = %d/%q/%+v, want the preset's crawl delay, list split and Kinder as family-friendly",
			c.RequestDelaySeconds, c.CategorySeparator, c.CategoryAttributes)
	}
}

func TestExampleInfomaxSourceParses(t *testing.T) {
	// Arrange
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "example-instance", "sources", "beispiel-infomax.example.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Act
	c, err := parseSource(data)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if c.CategoryFilter == nil || len(c.CategoryFilter.Deny) != 1 || len(c.Include) != 1 {
		t.Errorf("categoryFilter/include = %+v/%+v, want the example's filters", c.CategoryFilter, c.Include)
	}
}

func TestParseSourcePresetErrors(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"unknown preset":    {`{"preset": "nope"}`, `unknown preset "nope"`},
		"path in name":      {`{"preset": "../config"}`, `unknown preset`},
		"missing parameter": {`{"preset": "infomax", "params": {"host": "h"}}`, "missing parameter(s) permalink, widgetToken"},
		"unknown parameter": {`{"preset": "infomax", "params": {"host": "h", "permalink": "p", "widgetToken": "t", "tokn": "x"}}`, `no parameter "tokn"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseSource([]byte(tc.src))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestShippedPresetsAreComplete(t *testing.T) {
	presets, err := Presets()
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) == 0 {
		t.Fatal("no presets embedded")
	}
	for i := range presets {
		p := &presets[i]
		t.Run(p.Name, func(t *testing.T) {
			if p.Label == "" || p.Description == "" {
				t.Error("label and description are required (shown in the config UI)")
			}
			params := map[string]string{}
			for k, def := range p.Params {
				if def.Label == "" {
					t.Errorf("param %q has no label", k)
				}
				if def.Required && def.Example == "" {
					t.Errorf("required param %q has no example", k)
				}
				params[k] = def.Example
			}
			raw, err := p.render(params)
			if err != nil {
				t.Fatal(err)
			}
			var c SourceConfig
			if err := json.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			if c.Type == "" {
				t.Error("preset source sets no type")
			}
			// Every {name} left must be one the step runner or the mapping
			// fills - not an undeclared parameter.
			for k := range p.Params {
				if strings.Contains(string(raw), "{"+k+"}") {
					t.Errorf("placeholder {%s} not filled", k)
				}
			}
			urls := []string{c.URL}
			for _, st := range c.Steps {
				urls = append(urls, st.URL)
			}
			for _, m := range regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`).FindAllStringSubmatch(strings.Join(urls, " "), -1) {
				if m[1] != "today" && m[1] != "until" && m[1] != "item" && (c.ForEach == nil || m[1] != c.ForEach.Var) {
					found := false
					for _, st := range c.Steps {
						_, found = st.Collect[m[1]]
						if found {
							break
						}
					}
					if !found {
						t.Errorf("url placeholder {%s} is neither a parameter nor collected", m[1])
					}
				}
			}
		})
	}
}

func TestRecordAttemptKeepsPresetFileSmall(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "stralsund.json")
	if err := os.WriteFile(path, []byte(infomaxSource), 0o644); err != nil {
		t.Fatal(err)
	}

	// Act
	if err := RecordAttempt(path, nil); err != nil {
		t.Fatal(err)
	}

	// Assert
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"type", "url", "steps", "itemSelector"} {
		if _, ok := got[k]; ok {
			t.Errorf("%q written into the source file; preset fields must stay in the preset", k)
		}
	}
	for _, k := range []string{"preset", "params", "status", "extract"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%q missing after RecordAttempt", k)
		}
	}
	configs, err := Load(dir)
	if err != nil || len(configs) != 1 || configs[0].Type != "html" {
		t.Errorf("Load after RecordAttempt = %+v, %v", configs, err)
	}
}
