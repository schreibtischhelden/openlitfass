package writer

import (
	"path/filepath"
	"testing"
	"time"

	"openlitfass/backend/internal/model"
)

func TestCarryOverFailedSource(t *testing.T) {
	now := time.Now()
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	path := filepath.Join(t.TempDir(), "events.json")

	// Run 1: both sources fine.
	first := []model.Event{
		{Title: "Lesung", Date: tomorrow, StartTime: "19:00", Source: model.SourceRef{Name: "slow", SourceID: "1"}, LastSeen: now},
		{Title: "Konzert", Date: tomorrow, StartTime: "20:00", Source: model.SourceRef{Name: "ok", SourceID: "2"}, LastSeen: now},
	}
	run1, err := Write(path, first, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	lesungID := ""
	for _, e := range run1 {
		if e.Title == "Lesung" {
			lesungID = e.ID
		}
	}

	// Run 2: "slow" fails - its events come from the previous file.
	prev, err := LoadPrevious(path)
	if err != nil || len(prev) != 2 {
		t.Fatalf("LoadPrevious: %v, %d events", err, len(prev))
	}
	second := []model.Event{
		{Title: "Konzert", Date: tomorrow, StartTime: "20:00", Source: model.SourceRef{Name: "ok", SourceID: "2"}, LastSeen: now},
	}
	second = append(second, CarryOver(prev, map[string]bool{"slow": true}, today)...)
	run2, err := Write(path, second, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(run2) != 2 {
		t.Fatalf("want both events after a failed run, got %d", len(run2))
	}
	for _, e := range run2 {
		if e.Title == "Lesung" && (e.ID != lesungID || e.SourceName != "slow") {
			t.Errorf("carried event changed: id %q (was %q), source %q", e.ID, lesungID, e.SourceName)
		}
	}

	// Past events are not carried.
	old := []model.Event{{Title: "Vorbei", Date: "2000-01-01", SourceName: "slow", ID: "x"}}
	if got := CarryOver(old, map[string]bool{"slow": true}, today); len(got) != 0 {
		t.Errorf("past events must not be carried: %+v", got)
	}
}

func TestLoadPreviousMissingFile(t *testing.T) {
	if evs, err := LoadPrevious(filepath.Join(t.TempDir(), "nope.json")); err != nil || evs != nil {
		t.Errorf("missing file should be (nil, nil), got %v, %v", evs, err)
	}
}
