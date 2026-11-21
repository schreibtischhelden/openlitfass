package places

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openlitfass/backend/internal/model"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingIsNotAnError(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), FileName))
	if err != nil || f != nil {
		t.Errorf("got %v, %v; want nil, nil", f, err)
	}
}

func TestLoadValidates(t *testing.T) {
	types := `"types":[{"name":"Park","icon":["M0 0"]}]`
	for name, body := range map[string]string{
		"bad id":       `{` + types + `,"places":[{"id":"Großer See","name":"x","type":"Park"}]}`,
		"dup id":       `{` + types + `,"places":[{"id":"a","name":"x","type":"Park"},{"id":"a","name":"y","type":"Park"}]}`,
		"no name":      `{` + types + `,"places":[{"id":"a","type":"Park"}]}`,
		"unknown type": `{` + types + `,"places":[{"id":"a","name":"x","type":"Zoo"}]}`,
		"type no icon": `{"types":[{"name":"Park"}],"places":[]}`,
	} {
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	f, err := Load(write(t, `{`+types+`,"places":[{"id":"weisser-see","name":"Weißer See","type":"Park"}]}`))
	if err != nil || len(f.Places) != 1 {
		t.Fatalf("valid file: %v, %+v", err, f)
	}
}

func TestMatches(t *testing.T) {
	ev := func(name, addr string) model.Event {
		return model.Event{Location: model.Location{Name: name, Address: addr}}
	}
	delphi := Place{Name: "Theater im Delphi"}
	if !delphi.Matches(ev("THEATER IM DELPHI", "")) {
		t.Error("name match should ignore case")
	}
	brot := Place{Name: "Brotfabrik", Match: []string{"Brotfabrik", "Caligariplatz 1"}}
	if !brot.Matches(ev("KINO Brotfabrik", "")) || !brot.Matches(ev("Kino", "Caligariplatz 1, 13086 Berlin")) {
		t.Error("match list should hit name and address")
	}
	see := Place{Name: "Park am Weißen See", Match: []string{"Weißer See"}}
	if !see.Matches(ev("Am Weisser See", "")) {
		t.Error("ß and ss should be equal")
	}
	if delphi.Matches(ev("Brotfabrik", "")) {
		t.Error("unrelated place must not match")
	}
	if got := Fold("Straße Ü"); !strings.Contains(got, "strasse u") {
		t.Errorf("Fold = %q", got)
	}
}
