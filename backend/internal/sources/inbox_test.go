package sources

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestInboxRepeatOccurrences(t *testing.T) {
	r := inboxRepeat{
		Weekdays:  []string{"sa", "so"},
		From:      "2026-11-01",
		Until:     "2027-03-31",
		Dates:     []string{"2026-12-25", "2027-06-01"},
		Except:    []string{"2026-11-29"},
		DaysAhead: 14,
	}
	// Today is Saturday 2026-11-21; window 21.11.-05.12.
	got, last, err := r.occurrences("2026-11-21")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-11-21", "2026-11-22", "2026-11-28", "2026-12-05"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dates = %v, want %v", got, want)
	}
	if last != "2027-06-01" {
		t.Errorf("last = %s, want 2027-06-01 (latest of until and dates)", last)
	}
}

func TestInboxRepeatErrors(t *testing.T) {
	for name, r := range map[string]inboxRepeat{
		"no days or dates":   {},
		"weekdays w/o range": {Weekdays: []string{"mo"}},
		"unknown weekday":    {Weekdays: []string{"montag"}, From: "2026-01-01", Until: "2026-12-31"},
		"bad except":         {Dates: []string{"2026-12-01"}, Except: []string{"1.12."}},
	} {
		if _, _, err := r.occurrences("2026-11-21"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestInboxRepeatFile(t *testing.T) {
	dir := t.TempDir()
	file := `{
	  "title": "Hof geöffnet",
	  "startTime": "09:00", "endTime": "16:30",
	  "location": { "name": "Hof" },
	  "repeat": { "weekdays": ["mo", "di"], "from": "2000-01-03", "until": "2099-12-31", "daysAhead": 7 }
	}`
	if err := os.WriteFile(filepath.Join(dir, "hof.json"), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	// An expired repeating file is removed.
	old := `{ "title": "Alt", "repeat": { "dates": ["2000-01-01"] } }`
	if err := os.WriteFile(filepath.Join(dir, "alt.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	evs, err := NewInboxSource(dir).Fetch(nil)
	if err != nil {
		t.Fatal(err)
	}
	// 8 days (today..today+7) contain exactly 2 Mondays or Tuesdays... at
	// least 2, at most 3 occurrences depending on today's weekday.
	if len(evs) < 2 || len(evs) > 3 {
		t.Fatalf("got %d events, want 2-3", len(evs))
	}
	seen := map[string]bool{}
	for _, e := range evs {
		d, _ := time.Parse("2006-01-02", e.Date)
		if wd := d.Weekday(); wd != time.Monday && wd != time.Tuesday {
			t.Errorf("event on %s (%s)", e.Date, wd)
		}
		if e.StartTime != "09:00" || !e.Price.Free || e.Source.SourceID != "hof@"+e.Date {
			t.Errorf("event = %+v", e)
		}
		if seen[e.Source.SourceID] {
			t.Errorf("duplicate id %s", e.Source.SourceID)
		}
		seen[e.Source.SourceID] = true
	}
	if _, err := os.Stat(filepath.Join(dir, "alt.json")); err == nil {
		t.Error("expired repeat file should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "hof.json")); err != nil {
		t.Error("running repeat file must stay")
	}
}
