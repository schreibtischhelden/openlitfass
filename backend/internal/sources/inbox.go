package sources

import (
	"context"
	"encoding/json"
	"errors"
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
// the next run; see https://docs.openlitfass.org/?page=inbox for the file format) - the
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
	dir          string
	holidaysPath string
}

// NewInboxSource reads inbox files from dir; holidaysPath is the instance's
// holidays.json with the named day lists for "repeat" (may be missing).
func NewInboxSource(dir, holidaysPath string) *InboxSource {
	return &InboxSource{dir: dir, holidaysPath: holidaysPath}
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
func (s *InboxSource) Fetch(_ context.Context) ([]model.Event, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.dir, err)
	}
	// A broken holidays.json fails the whole source, so the previous run's
	// events are carried over instead of recurring events losing their
	// exceptions.
	lists, err := loadDayLists(s.holidaysPath)
	if err != nil {
		return nil, err
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

		evs, last, err := parseInboxFile(path, now, today, lists)
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
//	  "from": "2026-10-01", "until": "2027-03-31",  // or a yearly season: "04-01" .. "09-30"
//	  "dates": ["feiertage"],                        // additional days
//	  "except": ["12-24", "easter-2"],               // days to leave out
//	  "daysAhead": 60                                // how far ahead to generate (default 60)
//	}
//
// "dates" and "except" take day patterns (see dayPattern) and names of
// lists from <instance>/holidays.json. "until" is optional; without it the
// series is open-ended. "date" is not needed then. Each occurrence gets its
// own stable ID (<file name>@<date>). The file is deleted once "until" (or
// the last of "dates") has passed; open-ended, seasonal and pattern-based
// files never expire.
type inboxRepeat struct {
	Weekdays  []string `json:"weekdays"`
	From      string   `json:"from"`
	Until     string   `json:"until"`
	Dates     []string `json:"dates"`
	Except    []string `json:"except"`
	DaysAhead int      `json:"daysAhead"`
}

const defaultRepeatDaysAhead = 60

// neverExpires is the "last" date of a file that is never deleted
// automatically. It sorts after every real date.
const neverExpires = "9999-12-31"

var weekdayNames = map[string]time.Weekday{
	"so": time.Sunday, "su": time.Sunday, "sun": time.Sunday,
	"mo": time.Monday, "mon": time.Monday,
	"di": time.Tuesday, "tu": time.Tuesday, "tue": time.Tuesday,
	"mi": time.Wednesday, "we": time.Wednesday, "wed": time.Wednesday,
	"do": time.Thursday, "th": time.Thursday, "thu": time.Thursday,
	"fr": time.Friday, "fri": time.Friday,
	"sa": time.Saturday, "sat": time.Saturday,
}

// repeatRange is the period in which "weekdays" apply: either a fixed span
// (until may be open) or a season that recurs every year.
type repeatRange struct {
	seasonal        bool
	from, until     time.Time // fixed span; until is zero when open-ended
	fromMD, untilMD int       // season as month*100+day
}

func parseRepeatRange(from, until string) (repeatRange, error) {
	if from == "" {
		return repeatRange{}, errors.New(`repeat: "weekdays" needs "from"`)
	}
	if f, err := parseDayPattern(from); err == nil && f.kind == dayYearly {
		u, err := parseDayPattern(until)
		if until == "" || err != nil || u.kind != dayYearly {
			return repeatRange{}, fmt.Errorf(`repeat: seasonal "from" %q needs "until" as MM-DD too`, from)
		}
		return repeatRange{seasonal: true, fromMD: int(f.month)*100 + f.day, untilMD: int(u.month)*100 + u.day}, nil
	}
	var r repeatRange
	var err error
	if r.from, err = time.Parse("2006-01-02", from); err != nil {
		return r, fmt.Errorf(`repeat: invalid "from" %q (want YYYY-MM-DD or MM-DD)`, from)
	}
	if until != "" {
		if r.until, err = time.Parse("2006-01-02", until); err != nil {
			return r, fmt.Errorf(`repeat: invalid "until" %q (want YYYY-MM-DD, or MM-DD together with "from")`, until)
		}
	}
	return r, nil
}

func (r repeatRange) contains(d time.Time) bool {
	if r.seasonal {
		md := int(d.Month())*100 + d.Day()
		if r.fromMD <= r.untilMD {
			return md >= r.fromMD && md <= r.untilMD
		}
		return md >= r.fromMD || md <= r.untilMD // across the turn of the year
	}
	return !d.Before(r.from) && (r.until.IsZero() || !d.After(r.until))
}

// occurrences lists the dates of r between today and today+daysAhead, minus
// r.Except - sorted, and the last date the file is relevant for
// (neverExpires if it keeps producing dates).
func (r inboxRepeat) occurrences(today string, lists dayLists) (dates []string, last string, err error) {
	days := map[time.Weekday]bool{}
	for _, w := range r.Weekdays {
		d, ok := weekdayNames[strings.ToLower(strings.TrimSpace(w))]
		if !ok {
			return nil, "", fmt.Errorf("repeat: unknown weekday %q (use mo, di, mi, do, fr, sa, so)", w)
		}
		days[d] = true
	}
	if len(days) == 0 && len(r.Dates) == 0 {
		return nil, "", errors.New(`repeat: needs "weekdays" (with "from") or "dates"`)
	}
	extra, err := lists.expand(r.Dates)
	if err != nil {
		return nil, "", fmt.Errorf(`repeat: "dates": %w`, err)
	}
	except, err := lists.expand(r.Except)
	if err != nil {
		return nil, "", fmt.Errorf(`repeat: "except": %w`, err)
	}

	var rng repeatRange
	if len(days) > 0 {
		if rng, err = parseRepeatRange(r.From, r.Until); err != nil {
			return nil, "", err
		}
		if rng.seasonal || r.Until == "" {
			last = neverExpires
		} else {
			last = r.Until
		}
	}
	for _, p := range extra {
		if p.kind != dayOnce {
			last = neverExpires
		} else if s := fmt.Sprintf("%04d-%02d-%02d", p.year, p.month, p.day); s > last {
			last = s
		}
	}

	ahead := r.DaysAhead
	if ahead <= 0 {
		ahead = defaultRepeatDaysAhead
	}
	start, _ := time.Parse("2006-01-02", today)
	end := start.AddDate(0, 0, ahead)
	matchesAny := func(ps []dayPattern, d time.Time) bool {
		for _, p := range ps {
			if p.matches(d) {
				return true
			}
		}
		return false
	}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		in := (days[d.Weekday()] && rng.contains(d)) || matchesAny(extra, d)
		if in && !matchesAny(except, d) {
			dates = append(dates, d.Format("2006-01-02"))
		}
	}
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
func parseInboxFile(path string, now time.Time, today string, lists dayLists) (evs []model.Event, last string, err error) {
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
	dates, last, err := wrap.Repeat.occurrences(today, lists)
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
		return model.Event{}, errors.New(`missing "title"`)
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
