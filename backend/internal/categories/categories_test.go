package categories

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCats(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "categories.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadGroups(t *testing.T) {
	c, err := Load(writeCats(t, `{"fallback":"X","categories":[
	  {"name":"Kultur","icon":["M0 0"]},
	  {"name":"Kino","icon":["M0 0"],"group":"Kultur"},
	  {"name":"X","icon":["M0 0"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	entries := c.Build(nil)
	if entries[1].Name != "Kino" || entries[1].Group != "Kultur" || entries[0].Group != "" {
		t.Errorf("entries = %+v", entries)
	}
}

func TestLoadGroupErrors(t *testing.T) {
	for name, body := range map[string]string{
		"unknown group": `{"fallback":"X","categories":[{"name":"Kino","icon":["M"],"group":"Kultur"},{"name":"X","icon":["M"]}]}`,
		"self":          `{"fallback":"X","categories":[{"name":"Kino","icon":["M"],"group":"Kino"},{"name":"X","icon":["M"]}]}`,
		"nested":        `{"fallback":"X","categories":[{"name":"A","icon":["M"]},{"name":"B","icon":["M"],"group":"A"},{"name":"C","icon":["M"],"group":"B"},{"name":"X","icon":["M"]}]}`,
	} {
		if _, err := Load(writeCats(t, body)); err == nil || !strings.Contains(err.Error(), "group") {
			t.Errorf("%s: err = %v, want a group error", name, err)
		}
	}
}
