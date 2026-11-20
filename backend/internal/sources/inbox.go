package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

		ev, err := parseInboxFile(path, now)
		if err != nil {
			log.Printf("inbox %s: %v (left in place, fix and re-commit)", path, err)
			continue
		}

		if ev.Date < today {
			// Past its date - withCommonFilters would drop it anyway, so
			// there's nothing left for this file to contribute. Clean it up
			// instead of leaving it to accumulate indefinitely.
			if err := os.Remove(path); err != nil {
				log.Printf("inbox %s: event date %s has passed but couldn't remove file: %v", path, ev.Date, err)
			} else {
				log.Printf("inbox %s: event date %s has passed, removed", path, ev.Date)
			}
			continue
		}

		events = append(events, ev)
	}
	return events, nil
}

// parseInboxFile decodes one inbox file into a model.Event. The JSON shape
// mirrors model.Event's own public fields directly (title, description,
// category, date/startTime/endTime as plain "YYYY-MM-DD"/"HH:MM" strings -
// no timezone conversion, since a human is typing instance-local wall-clock
// time directly - location, price, organizer, url, ticketRequired,
// registrationRequired, familyFriendly). id/source/lastSeen are ignored if
// present and always set here.
func parseInboxFile(path string, now time.Time) (model.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.Event{}, fmt.Errorf("read: %w", err)
	}

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
	if _, err := time.Parse("2006-01-02", ev.Date); err != nil {
		return model.Event{}, fmt.Errorf(`invalid "date" (want YYYY-MM-DD): %w`, err)
	}

	// The filename (without extension) is this event's stable identity
	// across runs (see writer.MakeID) - if the file itself sets "id"
	// explicitly, writer.Write respects that instead (only fills it in if
	// still empty).
	base := strings.TrimSuffix(filepath.Base(path), ".json")
	ev.Source = model.SourceRef{Name: "inbox", SourceID: base}
	ev.LastSeen = now

	return ev, nil
}
