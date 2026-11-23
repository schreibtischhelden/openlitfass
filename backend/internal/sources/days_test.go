package sources

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEasterSunday(t *testing.T) {
	for year, want := range map[int]string{
		2024: "2024-03-31", 2025: "2025-04-20", 2026: "2026-04-05",
		2027: "2027-03-28", 2038: "2038-04-25", 2285: "2285-03-22",
	} {
		if got := easterSunday(year).Format("2006-01-02"); got != want {
			t.Errorf("easter %d = %s, want %s", year, got, want)
		}
	}
}

func TestParseDayPattern(t *testing.T) {
	for _, ok := range []string{"2027-05-06", "12-25", "02-29", "*-01", "*-31", "easter", "easter+39", "easter-2"} {
		if _, err := parseDayPattern(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "1.12.", "13-01", "12-32", "*-1", "*-32", "easter+", "easter+x", "eastern", "2027-02-30", "feiertage"} {
		if _, err := parseDayPattern(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

var testLists = dayLists{
	"ostern":    {"easter-2", "easter", "easter+1"},
	"feiertage": {"12-25", "12-26", "ostern"},
}

func TestInboxRepeatPatterns(t *testing.T) {
	for name, tc := range map[string]struct {
		r     inboxRepeat
		today string
		want  []string
		last  string
	}{
		"open end, yearly and monthly exceptions": {
			r:     inboxRepeat{Weekdays: []string{"di", "fr"}, From: "2026-11-01", Except: []string{"feiertage", "*-22"}, DaysAhead: 10},
			today: "2026-12-20", // Tue 22 (*-22), Fri 25 (12-25), Tue 29
			want:  []string{"2026-12-29"},
			last:  neverExpires,
		},
		"easter days left out": {
			r:     inboxRepeat{Weekdays: []string{"mo", "fr"}, From: "2027-01-01", Until: "2027-12-31", Except: []string{"ostern"}, DaysAhead: 10},
			today: "2027-03-20", // Mon 22, Good Friday 26, Easter Monday 29
			want:  []string{"2027-03-22"},
			last:  "2027-12-31",
		},
		"holidays added to weekends": {
			r:     inboxRepeat{Weekdays: []string{"sa", "so"}, From: "2026-10-01", Until: "2027-09-30", Dates: []string{"feiertage"}, DaysAhead: 10},
			today: "2026-12-20",
			want:  []string{"2026-12-20", "2026-12-25", "2026-12-26", "2026-12-27"},
			last:  neverExpires, // "feiertage" recurs every year
		},
		"season across the turn of the year": {
			r:     inboxRepeat{Weekdays: []string{"sa"}, From: "10-01", Until: "03-31", DaysAhead: 14},
			today: "2027-03-27",
			want:  []string{"2027-03-27"},
			last:  neverExpires,
		},
		"season within a year": {
			r:     inboxRepeat{Weekdays: []string{"sa"}, From: "04-01", Until: "09-30", DaysAhead: 14},
			today: "2027-03-27",
			want:  []string{"2027-04-03", "2027-04-10"},
			last:  neverExpires,
		},
		"single dates only": {
			r:     inboxRepeat{Dates: []string{"2026-12-24", "2027-01-10"}, DaysAhead: 10},
			today: "2026-12-20",
			want:  []string{"2026-12-24"},
			last:  "2027-01-10",
		},
	} {
		got, last, err := tc.r.occurrences(tc.today, testLists)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: dates = %v, want %v", name, got, tc.want)
		}
		if last != tc.last {
			t.Errorf("%s: last = %s, want %s", name, last, tc.last)
		}
	}
}

func TestInboxRepeatPatternErrors(t *testing.T) {
	for name, r := range map[string]inboxRepeat{
		"unknown list":         {Dates: []string{"ferien"}},
		"season without until": {Weekdays: []string{"sa"}, From: "04-01"},
		"season mixed":         {Weekdays: []string{"sa"}, From: "04-01", Until: "2027-09-30"},
		"bad until":            {Weekdays: []string{"sa"}, From: "2027-04-01", Until: "30.09."},
	} {
		if _, _, err := r.occurrences("2026-11-21", testLists); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoadDayLists(t *testing.T) {
	dir := t.TempDir()
	write := func(content string) string {
		path := filepath.Join(dir, "holidays.json")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	if lists, err := loadDayLists(filepath.Join(dir, "missing.json")); err != nil || lists != nil {
		t.Errorf("missing file: lists=%v err=%v, want none", lists, err)
	}
	lists, err := loadDayLists(write(`{"ostern": ["easter-2", "easter+1"], "feiertage": ["01-01", "ostern"], "kuchen": ["*-01"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := lists.expand([]string{"feiertage"}); len(got) != 3 {
		t.Errorf("feiertage expands to %d days, want 3", len(got))
	}

	for name, content := range map[string]string{
		"invalid json":  `{"ostern": "easter"}`,
		"cycle":         `{"a": ["b"], "b": ["a"]}`,
		"unknown entry": `{"feiertage": ["weihnachten"]}`,
		"name is a day": `{"easter": ["easter-2"]}`,
	} {
		if _, err := loadDayLists(write(content)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestInboxOpenEndedFileStays(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	if err := os.Mkdir(inbox, 0o755); err != nil {
		t.Fatal(err)
	}
	holidays := filepath.Join(root, "holidays.json")
	if err := os.WriteFile(holidays, []byte(`{"feiertage": ["12-25", "easter"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Started long ago, no "until": must neither expire nor be removed.
	file := `{ "title": "Markt", "repeat": { "weekdays": ["mo","di","mi","do","fr","sa","so"], "from": "2000-01-01", "except": ["feiertage"], "daysAhead": 3 } }`
	if err := os.WriteFile(filepath.Join(inbox, "markt.json"), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}

	evs, err := NewInboxSource(inbox, holidays).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) < 2 {
		t.Errorf("got %d events, want at least 2 of 4 days", len(evs))
	}
	if _, err := os.Stat(filepath.Join(inbox, "markt.json")); err != nil {
		t.Error("open-ended repeat file must stay")
	}

	if err := os.WriteFile(holidays, []byte(`{"feiertage": ["weihnachten"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewInboxSource(inbox, holidays).Fetch(context.Background()); err == nil {
		t.Error("a broken holidays.json should fail the source")
	}
}
