package sources

import (
	"os"
	"strings"
	"testing"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/model"
)

func icalWindow(from string, days int) (time.Time, time.Time) {
	f, _ := time.ParseInLocation("2006-01-02", from, model.Local)
	return f, f.AddDate(0, 0, days)
}

func cal(events ...string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" + strings.Join(events, "") + "END:VCALENDAR\r\n"
}

func TestICalEventsManagerExport(t *testing.T) {
	body, err := os.ReadFile("testdata/events-manager.ics")
	if err != nil {
		t.Fatal(err)
	}
	from, to := icalWindow("2026-11-20", 365)
	evs, err := parseICal(string(body), config.SourceConfig{Name: "fb"}, from, to, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	e := evs[0]
	checks := map[string][2]string{
		"title":    {e.Title, "Nach Haus – Die Seilschaft"},
		"date":     {e.Date, "2027-07-09"},
		"start":    {e.StartTime, "19:00"},
		"end":      {e.EndTime, ""}, // DTEND == DTSTART means "no end"
		"category": {e.Category, "Konzert"},
		"place":    {e.Location.Name, "Große Bühne"},
		"address":  {e.Location.Address, "Große Seestraße 10, Berlin-Weißensee, Berlin, 13086, Deutschland"},
		"url":      {e.URL, "https://freilichtbuehne-weissensee.de/events/nach-haus-die-seilschaft/"},
		"uid":      {e.Source.SourceID, "977@freilichtbuehne-weissensee.de"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if !strings.Contains(e.Description, "Gundermann auf der Freilichtbühne") || !strings.Contains(e.Description, "\n\n") {
		t.Errorf("description not unfolded/unescaped properly: %q", e.Description)
	}
	if e.Price.Free || e.Price.Amount != nil {
		t.Errorf("iCal has no price, want unknown, got %+v", e.Price)
	}
}

func TestICalTimeForms(t *testing.T) {
	body := cal(
		"BEGIN:VEVENT\r\nUID:utc\r\nSUMMARY:UTC\r\nDTSTART:20261201T170000Z\r\nDTEND:20261201T183000Z\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:tz\r\nSUMMARY:London\r\nDTSTART;TZID=Europe/London:20261202T100000\r\nDURATION:PT2H\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:float\r\nSUMMARY:Floating\r\nDTSTART:20261203T200000\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:allday\r\nSUMMARY:All day\r\nDTSTART;VALUE=DATE:20261204\r\nDTEND;VALUE=DATE:20261205\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:cancel\r\nSUMMARY:Cancelled\r\nSTATUS:CANCELLED\r\nDTSTART:20261205T100000\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:alarm\r\nSUMMARY:With alarm\r\nDTSTART:20261206T100000\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nSUMMARY:not the title\r\nEND:VALARM\r\nEND:VEVENT\r\n",
	)
	from, to := icalWindow("2026-11-20", 60)
	evs, _ := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())
	got := map[string]string{}
	for _, e := range evs {
		got[e.Source.SourceID] = e.Title + "|" + e.Date + "|" + e.StartTime + "|" + e.EndTime
		if e.Cancelled != (e.Source.SourceID == "cancel") {
			t.Errorf("%s: cancelled = %v", e.Source.SourceID, e.Cancelled)
		}
	}
	want := map[string]string{
		"utc":    "UTC|2026-12-01|18:00|19:30",    // 17:00Z = 18:00 Berlin (winter)
		"tz":     "London|2026-12-02|11:00|13:00", // 10:00 London = 11:00 Berlin
		"float":  "Floating|2026-12-03|20:00|",    // floating = instance time
		"allday": "All day|2026-12-04||",
		"alarm":  "With alarm|2026-12-06|10:00|",
		"cancel": "Cancelled|2026-12-05|10:00|",
	}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}
}

func TestICalRRule(t *testing.T) {
	body := cal(
		// Every Tuesday and Thursday 18:00, from 2026-11-03, except 2026-11-26.
		"BEGIN:VEVENT\r\nUID:weekly\r\nSUMMARY:Chor\r\nDTSTART;TZID=Europe/Berlin:20261103T180000\r\nDTEND;TZID=Europe/Berlin:20261103T193000\r\n"+
			"RRULE:FREQ=WEEKLY;BYDAY=TU,TH\r\nEXDATE;TZID=Europe/Berlin:20261126T180000\r\nEND:VEVENT\r\n",
		// Override: the 2026-12-01 occurrence moved to 19:00.
		"BEGIN:VEVENT\r\nUID:weekly\r\nSUMMARY:Chor (später)\r\nRECURRENCE-ID;TZID=Europe/Berlin:20261201T180000\r\n"+
			"DTSTART;TZID=Europe/Berlin:20261201T190000\r\nDTEND;TZID=Europe/Berlin:20261201T203000\r\nEND:VEVENT\r\n",
		// Monthly, 3 times in total.
		"BEGIN:VEVENT\r\nUID:monthly\r\nSUMMARY:Repair\r\nDTSTART:20261015T140000\r\nRRULE:FREQ=MONTHLY;COUNT=3\r\nEND:VEVENT\r\n",
	)
	from, to := icalWindow("2026-11-20", 14) // 20.11. - 03.12.
	evs, _ := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())
	var got []string
	for _, e := range evs {
		got = append(got, e.Date+" "+e.StartTime+" "+e.Title)
	}
	// 26.11. is an EXDATE, 01.12. 18:00 is replaced by the 19:00 override,
	// and the monthly series (15.10., 15.11., 15.12.) has nothing in the window.
	expect := map[string]bool{
		"2026-11-24 18:00 Chor":          true,
		"2026-12-03 18:00 Chor":          true,
		"2026-12-01 19:00 Chor (später)": true,
	}
	if len(got) != len(expect) {
		t.Fatalf("got %v", got)
	}
	for _, g := range got {
		if !expect[g] {
			t.Errorf("unexpected occurrence %q (got %v)", g, got)
		}
	}
	ids := map[string]bool{}
	for _, e := range evs {
		if ids[e.Source.SourceID] {
			t.Errorf("duplicate source id %q", e.Source.SourceID)
		}
		ids[e.Source.SourceID] = true
	}
	if !ids["weekly|20261201T170000"] {
		t.Errorf("override should keep the id of the replaced occurrence, ids: %v", ids)
	}
}

func TestICalUnsupportedRRuleFallsBack(t *testing.T) {
	body := cal("BEGIN:VEVENT\r\nUID:x\r\nSUMMARY:Nth\r\nDTSTART:20261201T100000\r\nRRULE:FREQ=MONTHLY;BYDAY=1MO\r\nEND:VEVENT\r\n")
	from, to := icalWindow("2026-11-20", 90)
	evs, _ := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())
	if len(evs) != 1 || evs[0].Date != "2026-12-01" {
		t.Fatalf("want only the first occurrence, got %+v", evs)
	}
}

func TestParseDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"PT1H30M": 90 * time.Minute,
		"P1D":     24 * time.Hour,
		"P1W":     7 * 24 * time.Hour,
		"P1DT2H":  26 * time.Hour,
		"-PT15M":  -15 * time.Minute,
	}
	for in, want := range tests {
		if got, ok := parseDuration(in); !ok || got != want {
			t.Errorf("parseDuration(%q) = %v,%v want %v", in, got, ok, want)
		}
	}
}
