package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"openlitfass/backend/internal/config"
)

// Benchmarks for the parsers of fetched content. Compare branches with
// scripts/check.sh bench (docs/backend.md, "Benchmarks").

func BenchmarkParseHTMLEvents(b *testing.B) {
	var cfg config.SourceConfig
	if err := json.Unmarshal([]byte(jeventsSource), &cfg); err != nil {
		b.Fatal(err)
	}
	cfg.Name = "bench"
	body, err := os.ReadFile("testdata/jevents-list.html")
	if err != nil {
		b.Fatal(err)
	}
	now := time.Now()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseHTMLEvents(string(body), cfg, now); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseICalExport(b *testing.B) {
	body, err := os.ReadFile("testdata/events-manager.ics")
	if err != nil {
		b.Fatal(err)
	}
	from, to := icalWindow("2026-11-20", 365)
	now := time.Now()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseICal(string(body), config.SourceConfig{Name: "bench"}, from, to, now); err != nil {
			b.Fatal(err)
		}
	}
}

// A calendar of 50 weekly series with exceptions over a year: the RRULE
// expansion is the expensive part of an ical source.
func BenchmarkParseICalRecurring(b *testing.B) {
	var events []string
	for i := range 50 {
		events = append(events, fmt.Sprintf(
			"BEGIN:VEVENT\r\nUID:series-%d\r\nSUMMARY:Kurs %d\r\n"+
				"DTSTART;TZID=Europe/Berlin:20261102T%02d0000\r\nDTEND;TZID=Europe/Berlin:20261102T%02d3000\r\n"+
				"RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR\r\nEXDATE;TZID=Europe/Berlin:20261225T%02d0000\r\nEND:VEVENT\r\n",
			i, i, 8+i%12, 8+i%12, 8+i%12))
	}
	body := cal(strings.Join(events, ""))
	from, to := icalWindow("2026-11-20", 365)
	now := time.Now()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseICal(body, config.SourceConfig{Name: "bench"}, from, to, now); err != nil {
			b.Fatal(err)
		}
	}
}
