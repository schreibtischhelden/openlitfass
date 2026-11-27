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

// Some exporters write every date of a series as a separate VEVENT with
// the same UID (no RRULE): each date must keep its own id.
func TestICalSharedUIDWithoutRRule(t *testing.T) {
	body := cal(
		"BEGIN:VEVENT\r\nUID:series\r\nSUMMARY:Spielrunde\r\nDTSTART:20261130T160000\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:series\r\nSUMMARY:Spielrunde\r\nDTSTART:20261207T160000\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:single\r\nSUMMARY:Einzeln\r\nDTSTART:20261201T100000\r\nEND:VEVENT\r\n",
	)
	from, to := icalWindow("2026-11-20", 60)
	evs, _ := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())
	ids := map[string]bool{}
	for _, e := range evs {
		ids[e.Source.SourceID] = true
	}
	if len(ids) != 3 || !ids["single"] {
		t.Errorf("ids = %v, want 3 distinct ids, plain UID kept for the single event", ids)
	}
}

func TestICalMultiDayAllDayEventRunsEveryDay(t *testing.T) {
	// Arrange: an exhibition that started before the window and is still running.
	body := cal("BEGIN:VEVENT\r\nUID:expo\r\nSUMMARY:Ausstellung\r\nDTSTART;VALUE=DATE:20261002\r\nDTEND;VALUE=DATE:20270101\r\nEND:VEVENT\r\n")
	from, to := icalWindow("2026-12-29", 30)

	// Act
	evs, err := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range evs {
		got = append(got, e.Date+" "+e.StartTime+" "+e.Source.SourceID)
	}
	want := []string{"2026-12-29  expo|2026-12-29", "2026-12-30  expo|2026-12-30", "2026-12-31  expo|2026-12-31"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestICalMultiDayTimedEventKeepsStartTimeOnFirstDay(t *testing.T) {
	// Arrange: a weekend festival Friday 18:00 to Sunday 14:00.
	body := cal("BEGIN:VEVENT\r\nUID:fest\r\nSUMMARY:Fest\r\nDTSTART:20261204T180000\r\nDTEND:20261206T140000\r\nEND:VEVENT\r\n")
	from, to := icalWindow("2026-11-27", 30)

	// Act
	evs, _ := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())

	// Assert
	var got []string
	for _, e := range evs {
		got = append(got, e.Date+" "+e.StartTime+"-"+e.EndTime)
	}
	want := []string{"2026-12-04 18:00-", "2026-12-05 -", "2026-12-06 -"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestICalShortEventsStaySingle(t *testing.T) {
	// Arrange: an overnight party and a one-day all-day event.
	body := cal(
		"BEGIN:VEVENT\r\nUID:party\r\nSUMMARY:Party\r\nDTSTART:20261204T220000\r\nDTEND:20261205T030000\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:day\r\nSUMMARY:Tag\r\nDTSTART;VALUE=DATE:20261210\r\nDTEND;VALUE=DATE:20261211\r\nEND:VEVENT\r\n",
	)
	from, to := icalWindow("2026-11-27", 30)

	// Act
	evs, _ := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())

	// Assert
	if len(evs) != 2 || evs[0].Source.SourceID != "party" || evs[1].Source.SourceID != "day" {
		t.Errorf("want two single events with plain ids, got %+v", evs)
	}
}

func TestICalMultiDayEventEndsAtWindow(t *testing.T) {
	// Arrange: a long exhibition beyond the window.
	body := cal("BEGIN:VEVENT\r\nUID:long\r\nSUMMARY:Lang\r\nDTSTART;VALUE=DATE:20261101\r\nDURATION:P365D\r\nEND:VEVENT\r\n")
	from, to := icalWindow("2026-11-27", 10)

	// Act
	evs, _ := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())

	// Assert
	if len(evs) != 10 || evs[0].Date != "2026-11-27" || evs[9].Date != "2026-12-06" {
		t.Errorf("want 10 days 2026-11-27..2026-12-06, got %d events", len(evs))
	}
}

// brokenExport mimics the TYPO3 tx_cal export of kirche-mv.de: content lines
// end with CRLF, but long values are wrapped with a bare LF (the space at
// the wrap is dropped), LOCATION parts sit on bare-LF + tab lines, quotes
// are written as \", ORGANIZER is a plain name and local times carry a "Z"
// after a fixed one-hour subtraction.
const brokenExport = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\nUID:1\r\n" +
	"LOCATION:* Dorfkirche Beispielhausen\n\t* Kirchweg 3 \n\t* 12345 Beispielhausen\r\n" +
	"SUMMARY:Konzert mit dem Chor \\\"Klangfarben\\\" und dem\nPosaunenchor\r\n" +
	"DESCRIPTION:Ein Abend mit Liedern zum Mitsingen. Weitere Infos unter\nhttps://example.org/konzert und vor Ort.\r\n" +
	"ORGANIZER:Kirchengemeinde Beispielhausen -\nwww.example.org\r\n" +
	"DTSTART:20261002T090000Z\r\nDTEND:20261002T100000Z\r\n" +
	"URL:https://example.org/veranstaltung/1\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:2\r\nLOCATION:* Gemeindehaus\r\nSUMMARY:Andacht\r\n" +
	"ORGANIZER:mailto:info@example.org\r\n" +
	"DTSTART:20261129T090000Z\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestICalLenientBrokenExport(t *testing.T) {
	from, to := icalWindow("2026-09-01", 120)
	evs, err := parseICal(brokenExport, config.SourceConfig{Name: "k", Organizer: "Fallback"}, from, to, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("want 2 events, got %d", len(evs))
	}
	e := evs[0]
	checks := map[string][2]string{
		"title":       {e.Title, `Konzert mit dem Chor "Klangfarben" und dem Posaunenchor`},
		"description": {e.Description, "Ein Abend mit Liedern zum Mitsingen. Weitere Infos unter https://example.org/konzert und vor Ort."},
		"organizer":   {e.Organizer, "Kirchengemeinde Beispielhausen - www.example.org"},
		"place":       {e.Location.Name, "Dorfkirche Beispielhausen"},
		"address":     {e.Location.Address, "Kirchweg 3, 12345 Beispielhausen"},
		"url":         {e.URL, "https://example.org/veranstaltung/1"},
		"start":       {e.StartTime, "11:00"}, // UTC trusted without icalUTCAs
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if got := evs[1]; got.Organizer != "Fallback" || got.Location.Name != "* Gemeindehaus" {
		t.Errorf("mailto ORGANIZER must not become a name, single-line LOCATION stays as is: %q, %q", got.Organizer, got.Location.Name)
	}
}

func TestICalUTCAs(t *testing.T) {
	from, to := icalWindow("2026-09-01", 120)
	tests := map[string][3]string{ // icalUTCAs -> October start, October end, November start
		"+01:00": {"10:00", "11:00", "10:00"},
		"+0100":  {"10:00", "11:00", "10:00"},
		"local":  {"09:00", "10:00", "09:00"},
	}
	for opt, want := range tests {
		evs, err := parseICal(brokenExport, config.SourceConfig{Name: "k", ICalUTCAs: opt}, from, to, time.Now())
		if err != nil {
			t.Fatalf("%s: %v", opt, err)
		}
		got := [3]string{evs[0].StartTime, evs[0].EndTime, evs[1].StartTime}
		if got != want {
			t.Errorf("icalUTCAs %s: got %v, want %v", opt, got, want)
		}
	}
	if _, err := parseICal(brokenExport, config.SourceConfig{Name: "k", ICalUTCAs: "MEZ"}, from, to, time.Now()); err == nil {
		t.Error("invalid icalUTCAs must be an error")
	}
}

func TestICalUnfoldModes(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"RFC folding", "SUMMARY:Lange\r\n  Zeile\r\nUID:1\r\n", []string{"SUMMARY:Lange Zeile", "UID:1"}},
		{"LF only stays strict", "SUMMARY:Erste\nZweite: Zeile\n", []string{"SUMMARY:Erste", "Zweite: Zeile", ""}},
		{"bare LF before a property", "SUMMARY:Titel\nUID:1\r\nEND:VEVENT\r\n", []string{"SUMMARY:Titel", "UID:1", "END:VEVENT"}},
		{"bare LF before X- property", "SUMMARY:Titel\nX-FOO:1\r\nUID:2\r\n", []string{"SUMMARY:Titel", "X-FOO:1", "UID:2"}},
		{"URL-like continuation", "DESCRIPTION:Mehr unter\nhttps://example.org\r\nUID:1\r\n", []string{"DESCRIPTION:Mehr unter https://example.org", "UID:1"}},
		{"label-like continuation", "SUMMARY:Konzert\nSopran: Anna\r\nUID:1\r\n", []string{"SUMMARY:Konzert Sopran: Anna", "UID:1"}},
	}
	for _, tt := range tests {
		got := unfold(tt.in)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%s: unfold = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestUnescapeText(t *testing.T) {
	tests := map[string]string{
		`a\,b\;c\nd`: "a,b;c\nd",
		`\"Zitat\"`:  `"Zitat"`,
		`C:\\Pfad`:   `C:\Pfad`,
		`\\"`:        `\"`,
	}
	for in, want := range tests {
		if got := unescapeText(in); got != want {
			t.Errorf("unescapeText(%q) = %q, want %q", in, got, want)
		}
	}
}
