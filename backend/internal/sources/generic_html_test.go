package sources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/htmlsel"
)

// A source file as an instance would write it (see https://docs.openlitfass.org/?page=source-types#html).
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

func TestExtractRegexAlternatives(t *testing.T) {
	cfg := config.SourceConfig{
		Name: "t", URL: "https://x.example/", ItemSelector: ".e",
		Extract: map[string]config.ExtractRule{
			"price": {Selector: "p", Regex: `(?i)Tickets:\s*(?:ab\s*)?(\d+(?:,\d+)?)\s*€|eintritt\s+(frei)`},
			"title": {Selector: "h2"},
		},
	}
	body := `<div class="e"><h2>A</h2><p>Tickets: Ab 27,00€</p></div>
<div class="e"><h2>B</h2><p>Eintritt frei!</p></div>
<div class="e"><h2>C</h2><p>Tickets: Ab 1. Oktober erhältlich!</p></div>`
	for i, want := range []string{"27,00", "frei", ""} {
		r := compiledExtract{rule: cfg.Extract["price"]}
		r.re = regexp.MustCompile(cfg.Extract["price"].Regex)
		sel := htmlsel.MustParse("p")
		r.sel = &sel
		doc, _ := html.Parse(strings.NewReader(body))
		item := htmlsel.MustParse(".e").QueryAll(doc)[i]
		if got := extractValue(item, r, nil); got != want {
			t.Errorf("item %d: %q, want %q", i, got, want)
		}
	}
}

func TestRecordAttemptKeepsSelectorsReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{"type":"html","itemSelector":".row > div","extract":{"t":"a > b"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.RecordAttempt(path, nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), `\u003e`) || !strings.Contains(string(b), `".row > div"`) {
		t.Errorf("source file got escaped:\n%s", b)
	}
}

// Date headings followed by sibling entries (no container per day): each
// entry gets the last heading before it as "section".
func TestHTMLSourceSectionSelector(t *testing.T) {
	page := `<html><body><aside>
	  <h2>Programm</h2>
	  <h3 class="day">Samstag, 21.11.2026</h3>
	  <div class="entry"><a href="/film/a/1">12:45 Film A</a></div>
	  <div class="entry"><a href="/film/b/2">20:15 Film B</a></div>
	  <h3 class="day">Sonntag, 22.11.2026</h3>
	  <div class="entry"><a href="/film/a/3">11:00 Film A</a></div>
	</aside></body></html>`
	var cfg config.SourceConfig
	if err := json.Unmarshal([]byte(`{
	  "name": "kino", "type": "html", "url": "https://kino.example/",
	  "itemSelector": "div.entry", "sectionSelector": "h3.day",
	  "extract": {
	    "time": { "selector": "a", "regex": "^([0-9]{1,2}:[0-9]{2})" },
	    "title": { "selector": "a", "regex": "^[0-9]{1,2}:[0-9]{2} +(.+)$" },
	    "link": { "selector": "a", "attr": "href" }
	  },
	  "mapping": {
	    "title": { "path": "title" },
	    "url": { "path": "link" },
	    "start": { "template": "{section} {time}", "regex": "([0-9]{2}[.][0-9]{2}[.][0-9]{4} [0-9]{1,2}:[0-9]{2})", "format": "02.01.2006 15:04" }
	  }
	}`), &cfg); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	evs, err := parseHTMLEvents(page, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ title, date, time, url string }{
		{"Film A", "2026-11-21", "12:45", "https://kino.example/film/a/1"},
		{"Film B", "2026-11-21", "20:15", "https://kino.example/film/b/2"},
		{"Film A", "2026-11-22", "11:00", "https://kino.example/film/a/3"},
	}
	if len(evs) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(evs), len(want), evs)
	}
	for i, w := range want {
		e := evs[i]
		if e.Title != w.title || e.Date != w.date || e.StartTime != w.time || e.URL != w.url {
			t.Errorf("event %d = %q %s %s %s, want %+v", i, e.Title, e.Date, e.StartTime, e.URL, w)
		}
	}
}
