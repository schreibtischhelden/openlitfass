package writer

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openlitfass/backend/internal/model"
)

func ev(source, title, date, start string) model.Event {
	return model.Event{
		Title: title, Date: date, StartTime: start,
		Price:  model.Price{Free: false, Amount: nil},
		Source: model.SourceRef{Name: source, SourceID: title + date + start},
	}
}

func TestNormalizeTitle(t *testing.T) {
	for in, want := range map[string]string{
		`„Langsam Langsam!" – oder die Schnecke`:                "langsam langsam oder die schnecke",
		"Rudelsingen - Das Original aus Münster [AUSVERKAUFT!]": "rudelsingen das original aus muenster",
		"RUDELSINGEN – das Original aus Muenster":               "rudelsingen das original aus muenster",
		"Straßenfest (Kinder 3-6)":                              "strassenfest kinder 3 6",
	} {
		if got := NormalizeTitle(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestMergeAcrossSources(t *testing.T) {
	d := "2027-01-10"
	venue := ev("peter-edel", "Rudelsingen - Das Original aus Münster [AUSVERKAUFT!]", d, "20:00")
	venue.Location = model.Location{Name: "Peter Edel"}
	agg := ev("berlin-de", "RUDELSINGEN – das Original aus Muenster", d, "20:00")
	agg.Location = model.Location{Name: "Bildungs- und Kulturzentrum Peter Edel", Address: "Berliner Allee 125, 13088 Berlin"}
	agg.Description = "Singen im Rudel."
	price := 18.0
	agg.Price = model.Price{Amount: &price}
	agg.Cancelled = true

	other := ev("brotfabrik", "Übergang", d, "20:00") // same time, different event

	out := MergeAcrossSources([]model.Event{agg, venue, other}, map[string]int{"peter-edel": 10})
	if len(out) != 2 {
		t.Fatalf("want 2 events, got %d: %+v", len(out), out)
	}
	m := out[0]
	if m.Source.Name != "peter-edel" || m.Title != venue.Title {
		t.Errorf("higher priority source should win, got %s %q", m.Source.Name, m.Title)
	}
	if strings.Join(m.Sources, ",") != "peter-edel,berlin-de" {
		t.Errorf("sources = %v", m.Sources)
	}
	if m.Location.Name != "Peter Edel" || m.Location.Address != agg.Location.Address {
		t.Errorf("location should keep the winner's name and fill the address: %+v", m.Location)
	}
	if m.Description != "Singen im Rudel." || m.Price.Amount == nil || *m.Price.Amount != 18 {
		t.Errorf("gaps not filled: %q %+v", m.Description, m.Price)
	}
	if !m.Cancelled {
		t.Error("a cancellation from any source should win")
	}
	if len(out[1].Sources) != 1 || out[1].Sources[0] != "brotfabrik" {
		t.Errorf("unmerged event should list its own source: %v", out[1].Sources)
	}
}

func TestMergeRespectsPostcodes(t *testing.T) {
	a := ev("a", "Kiezcafé", "2027-01-10", "16:00")
	a.Location.Address = "Pistoriusstraße 23, 13086 Berlin"
	b := ev("b", "Kiezcafé", "2027-01-10", "16:00")
	b.Location.Address = "Musterweg 1, 10437 Berlin"
	if out := MergeAcrossSources([]model.Event{a, b}, nil); len(out) != 2 {
		t.Errorf("different postcodes must not be merged, got %d", len(out))
	}
}

func TestMergeNeverWithinOneSource(t *testing.T) {
	a := ev("fz", "Yoga", "2027-01-10", "18:00")
	a.Location.Name = "Raum 1"
	b := ev("fz", "Yoga", "2027-01-10", "18:00")
	b.Location.Name = "Raum 2"
	b.Source.SourceID = "other"
	out, err := Write(filepath.Join(t.TempDir(), "e.json"), []model.Event{a, b}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].ID == out[1].ID {
		t.Fatalf("two rooms of one source must stay two events with distinct ids: %+v", out)
	}
	if !strings.HasSuffix(out[1].ID, "-2") {
		t.Errorf("second id should get a deterministic suffix, got %q", out[1].ID)
	}
}

func TestEventIDIsSourceIndependent(t *testing.T) {
	now := time.Now()
	d := now.Format("2006-01-02")
	run := func(src string) string {
		e := ev(src, "Lesung im Hof", d, "19:00")
		e.LastSeen = now
		out, err := Write(filepath.Join(t.TempDir(), "e.json"), []model.Event{e}, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].ID
	}
	a, b := run("venue"), run("aggregator")
	if a != b || !strings.HasPrefix(a, "lesung-im-hof-") {
		t.Errorf("ids %q / %q should be equal and readable", a, b)
	}
}

func TestMergeFillsPlaceReference(t *testing.T) {
	// Arrange
	venue := ev("venue", "Konzert", "2030-01-01", "20:00")
	aggregator := ev("aggregator", "Konzert", "2030-01-01", "20:00")
	aggregator.Place = "vow:5809"

	// Act
	got := MergeAcrossSources([]model.Event{venue, aggregator}, map[string]int{"venue": 1})

	// Assert
	if len(got) != 1 || got[0].Place != "vow:5809" {
		t.Errorf("merged = %+v", got)
	}
}
