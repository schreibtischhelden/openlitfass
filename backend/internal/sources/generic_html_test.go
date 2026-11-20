package sources

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"openlitfass/backend/internal/config"
)

// A source file as an instance would write it (see docs/backend.md).
const jeventsSource = `{
  "type": "html",
  "url": "https://test.example/veranstaltungen.html",
  "itemSelector": "#jevents_body .schatten",
  "extract": {
    "day": ".neulandeventDate .day",
    "month": ".neulandeventDate .month",
    "year": ".neulandeventDate .year",
    "title": ".headline h2",
    "category": "a.ev_link_cat",
    "categoryUrl": { "selector": "a.ev_link_cat", "attr": "href" },
    "start": { "selector": ".headline p:nth-of-type(2)", "regex": "(\\d{1,2}:\\d{2})" },
    "end": { "selector": ".headline p:nth-of-type(2)", "regex": "-\\s*(\\d{1,2}:\\d{2})" },
    "place": ".headline p:nth-of-type(3) strong",
    "description": { "selector": ".headline p", "all": true, "skip": 3, "html": true }
  },
  "mapping": {
    "title": { "path": "title" },
    "description": { "path": "description" },
    "category": { "path": "category" },
    "url": { "default": "https://test.example/veranstaltungen.html" },
    "locationName": { "path": "place", "default": "Testzentrum" },
    "locationAddress": { "default": "Teststraße 1, 12345 Berlin" },
    "start": { "template": "{day} {month} {year} {start}", "format": "2 Jan 2006 15:04|2 Jan 2006" },
    "end": { "template": "{day} {month} {year} {end}", "format": "2 Jan 2006 15:04" }
  }
}`

func TestHTMLSourceJEventsList(t *testing.T) {
	var cfg config.SourceConfig
	if err := json.Unmarshal([]byte(jeventsSource), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Name = "test"
	body, err := os.ReadFile("testdata/jevents-list.html")
	if err != nil {
		t.Fatal(err)
	}
	evs, err := parseHTMLEvents(string(body), cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 {
		t.Fatalf("want 3 events (nav .schatten must not count), got %d", len(evs))
	}

	e := evs[0]
	for name, c := range map[string][2]string{
		"title":   {e.Title, "Spieleabend für alle"},
		"date":    {e.Date, "2026-12-04"},
		"start":   {e.StartTime, "18:00"},
		"end":     {e.EndTime, "21:00"},
		"cat":     {e.Category, "Spielen"},
		"place":   {e.Location.Name, "Testzentrum, Saal"},
		"address": {e.Location.Address, "Teststraße 1, 12345 Berlin"},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if e.Description != "Brettspiele & Kartenspiele für alle Altersgruppen.\nKosten: 2 €\n\nAnmeldung: nicht erforderlich" {
		t.Errorf("description = %q", e.Description)
	}

	// Cancelled marker in the title is detected by the common filters, not
	// here - parseHTMLEvents returns the raw title.
	if evs[1].Title != "ENTFÄLLT - Tanznachmittag" || evs[1].Location.Name != "Testzentrum" {
		t.Errorf("event 2: %q at %q", evs[1].Title, evs[1].Location.Name)
	}
	if evs[2].Date != "2027-01-10" || evs[2].StartTime != "18:30" || evs[2].EndTime != "" {
		t.Errorf("event 3: %s %q-%q", evs[2].Date, evs[2].StartTime, evs[2].EndTime)
	}
}

func TestHTMLSourceNoMatchIsAnError(t *testing.T) {
	cfg := config.SourceConfig{Name: "t", URL: "https://x.example/", ItemSelector: ".gone"}
	if _, err := parseHTMLEvents("<html><body><p>neu gestaltet</p></body></html>", cfg, time.Now()); err == nil {
		t.Error("a selector that matches nothing should be reported (site redesign), not silently return zero events")
	}
}

func TestExtractRuleShorthandRoundTrip(t *testing.T) {
	var m map[string]config.ExtractRule
	in := `{"a":"h2","b":{"selector":"a","attr":"href"}}`
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(m)
	if string(out) != in {
		t.Errorf("round trip: %s", out)
	}
}
