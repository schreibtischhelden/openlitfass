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

func TestLoadValidatesRefs(t *testing.T) {
	types := `"types":[{"name":"Park","icon":["M0 0"]}]`
	for name, places := range map[string]string{
		"empty ref":      `[{"id":"a","name":"x","type":"Park","refs":[""]}]`,
		"padded ref":     `[{"id":"a","name":"x","type":"Park","refs":[" vow:1"]}]`,
		"ref is an id":   `[{"id":"a","name":"x","type":"Park"},{"id":"b","name":"y","type":"Park","refs":["a"]}]`,
		"ref used twice": `[{"id":"a","name":"x","type":"Park","refs":["vow:1"]},{"id":"b","name":"y","type":"Park","refs":["vow:1"]}]`,
	} {
		if _, err := Load(write(t, `{`+types+`,"places":`+places+`}`)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestPlaceForResolutionOrder(t *testing.T) {
	// Arrange
	f, err := Load(write(t, `{"types":[{"name":"Treff","icon":["M0 0"]}],"places":[
	  {"id":"kubiz","name":"KuBiZ","type":"Treff","match":["Bernkasteler Str. 78"]},
	  {"id":"bike-kitchen","name":"Bike Kitchen","type":"Treff","refs":["vow:5809"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	at := func(place, name string) model.Event {
		return model.Event{Place: place, Location: model.Location{Name: name}}
	}
	tests := []struct {
		name      string
		ev        model.Event
		wantID    string
		wantKnown bool
	}{
		{"ref beats match terms", at("vow:5809", "Bernkasteler Str. 78"), "bike-kitchen", true},
		{"place id works as reference", at("kubiz", "Woanders"), "kubiz", true},
		{"unknown ref falls back to match terms", at("vow:1", "Bernkasteler Str. 78"), "kubiz", false},
		{"no ref uses match terms", at("", "Bernkasteler Str. 78"), "kubiz", true},
		{"nothing fits", at("", "Woanders"), "", true},
	}
	for _, tt := range tests {
		// Act
		p, known := f.PlaceFor(tt.ev)

		// Assert
		got := ""
		if p != nil {
			got = p.ID
		}
		if got != tt.wantID || known != tt.wantKnown {
			t.Errorf("%s: got %q, known %t; want %q, %t", tt.name, got, known, tt.wantID, tt.wantKnown)
		}
	}
}

func TestAssignSetsPlaceIDAndReportsUnknownRefsOnce(t *testing.T) {
	// Arrange
	f := &File{Places: []Place{{ID: "hof", Name: "Hof", Refs: []string{"osm:1"}}}}
	events := []model.Event{
		{Place: "osm:1"},
		{Place: "vow:9", Location: model.Location{Name: "Am Hof"}},
		{Place: "vow:9"},
		{Place: "vow:2", PlaceID: "stale"},
	}

	// Act
	unknown := f.Assign(events)

	// Assert
	for i, want := range []string{"hof", "hof", "", ""} {
		if events[i].PlaceID != want {
			t.Errorf("event %d: placeId %q, want %q", i, events[i].PlaceID, want)
		}
	}
	if strings.Join(unknown, ",") != "vow:2,vow:9" {
		t.Errorf("unknown refs = %v", unknown)
	}
}
