package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"openlitfass/backend/internal/model"
)

// InboxSource turns hand-authored JSON files in the instance's inbox/
// directory into events (commit a file, it shows up in data/events.json on
// the next run; see docs/inbox.md for the file format) - the
// manual counterpart to the automated rss/json/wcs sources, for one-off
// events that don't come from any feed (a flyer, a phone call, a personal
// tip).
//
// Unlike every other Source, it has a side effect: once an event's date is
// in the past - the same point at which withCommonFilters would drop it
// from the output anyway - Fetch deletes the file that produced it, so the
// inbox directory cleans itself up over time instead of accumulating stale
// entries forever.
//
// That cleanup is deliberately tied to "this event is now in the past",
// not to "this file was processed once": cmd/fetch recomputes
// data/events.json fresh from every source's current Fetch() result on
// every run, nothing is merged in from the previous file. Deleting the
// file right after the first successful run would make the event vanish
// from the very next run - there'd be no source left to reproduce it -
// instead of staying visible until its date actually passes. Keeping the
// file in place and re-reading it every run is what makes the event
// persist; date-based deletion is the earliest point where removing the
// file has no visible effect.
type InboxSource struct {
	dir string
}

func NewInboxSource(dir string) *InboxSource {
	return &InboxSource{dir: dir}
}

func (s *InboxSource) Name() string { return "inbox" }

// Fetch reads every *.json file in dir (skipping *.example.json templates),
// parses each into a model.Event, and deletes any whose event date has
// already passed. A missing directory yields zero events, not an error -
// same convention as config.Load for <instance>/sources/.
//
// A single malformed file is logged and left in place (not deleted, so
// nothing is silently lost) rather than failing the whole run - the same
// resilience pattern used throughout internal/config and internal/sources
// for other kinds of bad input.
func (s *InboxSource) Fetch(ctx context.Context) ([]model.Event, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.dir, err)
	}

	today, _ := model.LocalParts(time.Now())
	now := time.Now().UTC()

	var events []model.Event
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".example.json") {
			continue
		}
		path := filepath.Join(s.dir, name)

		evs, last, err := parseInboxFile(path, now, today)
		if err != nil {
			log.Printf("inbox %s: %v (left in place, fix and re-commit)", path, err)
			continue
		}

		if last < today {
			// Past its (last) date - withCommonFilters would drop it anyway,
			// so there's nothing left for this file to contribute. Clean it
			// up instead of leaving it to accumulate indefinitely.
			if err := os.Remove(path); err != nil {
				log.Printf("inbox %s: last date %s has passed but couldn't remove file: %v", path, last, err)
			} else {
				log.Printf("inbox %s: last date %s has passed, removed", path, last)
			}
			continue
		}

		events = append(events, evs...)
	}
	return events, nil
}

// inboxRepeat turns one inbox file into recurring events (opening hours,
// a weekly workshop, ...) instead of a single one:
//
//	"repeat": {
//	  "weekdays": ["mo", "di", "mi", "do", "fr"],
//	  "from": "2026-10-01", "until": "2027-03-31",
//	  "dates": ["2026-12-25"],        // additional single dates (holidays)
//	  "except": ["2026-12-24"],       // dates to leave out
//	  "daysAhead": 60                 // how far ahead to generate (default 60)
//	}
//
// "date" is not needed then. Each occurrence gets its own stable ID
// (<file name>@<date>). The file is deleted once "until" (or the last of
// "dates") has passed.
type inboxRepeat struct {
	Weekdays  []string `json:"weekdays"`
	From      string   `json:"from"`
	Until     string   `json:"until"`
	Dates     []string `json:"dates"`
	Except    []string `json:"except"`
	DaysAhead int      `json:"daysAhead"`
}

const defaultRepeatDaysAhead = 60

var weekdayNames = map[string]time.Weekday{
	"so": time.Sunday, "su": time.Sunday, "sun": time.Sunday,
	"mo": time.Monday, "mon": time.Monday,
	"di": time.Tuesday, "tu": time.Tuesday, "tue": time.Tuesday,
	"mi": time.Wednesday, "we": time.Wednesday, "wed": time.Wednesday,
	"do": time.Thursday, "th": time.Thursday, "thu": time.Thursday,
	"fr": time.Friday, "fri": time.Friday,
	"sa": time.Saturday, "sat": time.Saturday,
}

// occurrences lists the dates of r between today and today+daysAhead (and
// within from..until), plus r.Dates in that window, minus r.Except -
// sorted, and the last date the file is relevant for.
func (r inboxRepeat) occurrences(today string) (dates []string, last string, err error) {
	parse := func(field, s string) (time.Time, error) {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return t, fmt.Errorf(`repeat: invalid %q %q (want YYYY-MM-DD)`, field, s)
		}
		return t, nil
	}
	days := map[time.Weekday]bool{}
	for _, w := range r.Weekdays {
		d, ok := weekdayNames[strings.ToLower(strings.TrimSpace(w))]
		if !ok {
			return nil, "", fmt.Errorf("repeat: unknown weekday %q (use mo, di, mi, do, fr, sa, so)", w)
		}
		days[d] = true
	}
	if len(days) > 0 && (r.From == "" || r.Until == "") {
		return nil, "", fmt.Errorf(`repeat: "weekdays" needs "from" and "until"`)
	}
	if len(days) == 0 && len(r.Dates) == 0 {
		return nil, "", fmt.Errorf(`repeat: needs "weekdays" (with "from"/"until") or "dates"`)
	}
	except := map[string]bool{}
	for _, d := range r.Except {
		if _, err := parse("except", d); err != nil {
			return nil, "", err
		}
		except[d] = true
	}

	ahead := r.DaysAhead
	if ahead <= 0 {
		ahead = defaultRepeatDaysAhead
	}
	start, _ := time.Parse("2006-01-02", today)
	end := start.AddDate(0, 0, ahead)

	set := map[string]bool{}
	if len(days) > 0 {
		from, err := parse("from", r.From)
		if err != nil {
			return nil, "", err
		}
		until, err := parse("until", r.Until)
		if err != nil {
			return nil, "", err
		}
		last = r.Until
		if from.Before(start) {
			from = start
		}
		if until.After(end) {
			until = end
		}
		for d := from; !d.After(until); d = d.AddDate(0, 0, 1) {
			if days[d.Weekday()] {
				set[d.Format("2006-01-02")] = true
			}
		}
	}
	for _, s := range r.Dates {
		d, err := parse("dates", s)
		if err != nil {
			return nil, "", err
		}
		if s > last {
			last = s
		}
		if !d.Before(start) && !d.After(end) {
			set[s] = true
		}
	}
	for d := range set {
		if !except[d] {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	return dates, last, nil
}

// parseInboxFile decodes one inbox file into a model.Event. The JSON shape
// mirrors model.Event's own public fields directly (title, description,
// category, date/startTime/endTime as plain "YYYY-MM-DD"/"HH:MM" strings -
// no timezone conversion, since a human is typing instance-local wall-clock
// time directly - location, price, organizer, url, ticketRequired,
// registrationRequired, familyFriendly). id/source/lastSeen are ignored if
// present and always set here.
//
// With "repeat" (see inboxRepeat) the file yields one event per occurrence.
// last is the date after which the file can be deleted.
func parseInboxFile(path string, now time.Time, today string) (evs []model.Event, last string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read: %w", err)
	}
	var wrap struct {
		Repeat *inboxRepeat `json:"repeat"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, "", fmt.Errorf("invalid json: %w", err)
	}
	if wrap.Repeat == nil {
		ev, err := parseInboxEvent(data, path, now, true)
		if err != nil {
			return nil, "", err
		}
		return []model.Event{ev}, ev.Date, nil
	}
	tmpl, err := parseInboxEvent(data, path, now, false)
	if err != nil {
		return nil, "", err
	}
	dates, last, err := wrap.Repeat.occurrences(today)
	if err != nil {
		return nil, "", err
	}
	for _, d := range dates {
		ev := tmpl
		ev.Date = d
		ev.Source.SourceID = tmpl.Source.SourceID + "@" + d
		evs = append(evs, ev)
	}
	return evs, last, nil
}

// parseInboxEvent decodes the event fields of one inbox file; needDate is
// false for repeating files (their dates come from "repeat").
func parseInboxEvent(data []byte, path string, now time.Time, needDate bool) (model.Event, error) {

	// Decoded separately (not just via the model.Event unmarshal below) so
	// we can tell "price omitted entirely" apart from "price explicitly
	// {}" - both produce the same Go zero value model.Price{Free: false,
	// Amount: nil}, which the frontend renders as "null €" (see
	// internal/model/event.go). Omitting price should mean "kostenlos" by
	// default, the same convention mapping.PriceRule uses when no amount
	// path is configured.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return model.Event{}, fmt.Errorf("invalid json: %w", err)
	}

	var ev model.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		return model.Event{}, fmt.Errorf("invalid json: %w", err)
	}
	if _, ok := raw["price"]; !ok {
		ev.Price = model.Price{Free: true, Amount: nil}
	}

	if strings.TrimSpace(ev.Title) == "" {
		return model.Event{}, fmt.Errorf(`missing "title"`)
	}
	if needDate {
		if _, err := time.Parse("2006-01-02", ev.Date); err != nil {
			return model.Event{}, fmt.Errorf(`invalid "date" (want YYYY-MM-DD): %w`, err)
		}
	}

	// The filename (without extension) is this event's stable identity
	// across runs within the inbox (Dedupe) - if the file itself sets "id"
	// explicitly, writer.Write respects that instead (only fills it in if
	// still empty).
	base := strings.TrimSuffix(filepath.Base(path), ".json")
	ev.Source = model.SourceRef{Name: "inbox", SourceID: base}
	ev.LastSeen = now

	return ev, nil
}
