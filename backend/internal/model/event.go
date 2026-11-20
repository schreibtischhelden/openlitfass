// Package model defines the normalized event schema shared by every source
// and written out as JSON for the frontend to consume.
//
// The shape of Event is dictated by the existing frontend (web/js/app.js reads
// ev.title, ev.description, ev.category, ev.date, ev.startTime, ev.endTime,
// ev.location.name, ev.price.free/.amount) — keep them in sync if either
// side changes.
package model

import (
	"time"

	// Embeds the IANA time zone database into the binary so time.LoadLocation
	// works even on minimal images (e.g. golang:alpine) that lack tzdata.
	_ "time/tzdata"
)

// DefaultTimezone is used when an instance doesn't set "timezone" in its
// instance.json.
const DefaultTimezone = "Europe/Berlin"

// Local is the instance's timezone: every event's Date/StartTime/EndTime is
// wall-clock time in this zone. Exported so the mapping package can
// time.ParseInLocation raw source timestamps directly into local wall-clock
// time (see internal/mapping). Set once at startup via SetTimezone (see
// cmd/fetch), before any source runs; defaults to DefaultTimezone.
var Local = mustLoad(DefaultTimezone)

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// Should be unreachable thanks to the time/tzdata import above, but
		// fall back to UTC rather than panicking if it ever happens.
		return time.UTC
	}
	return loc
}

// SetTimezone switches Local to the IANA zone name (e.g. "Europe/Vienna").
// An empty name keeps the current value.
func SetTimezone(name string) error {
	if name == "" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return err
	}
	Local = loc
	return nil
}

// LocalParts converts t to the instance timezone (Local) and splits it into
// the date (YYYY-MM-DD) and clock time (HH:MM) strings the frontend expects
// for Event.Date / Event.StartTime / Event.EndTime.
func LocalParts(t time.Time) (date, clock string) {
	lt := t.In(Local)
	return lt.Format("2006-01-02"), lt.Format("15:04")
}

// Location describes where an event takes place.
type Location struct {
	Name    string   `json:"name"`
	Address string   `json:"address,omitempty"`
	Lat     *float64 `json:"lat,omitempty"`
	Lng     *float64 `json:"lng,omitempty"`
}

// Price describes ticket cost. Amount is in euros; Free events should still
// set Amount to nil explicitly (it marshals to JSON null).
type Price struct {
	Free   bool     `json:"free"`
	Amount *float64 `json:"amount"`
}

// SourceRef identifies which fetcher produced an event and the event's id
// within that source. Not part of the public JSON — used internally to
// derive a stable Event.ID and to dedupe across runs.
type SourceRef struct {
	Name     string
	SourceID string
}

// Event is the normalized, frontend-ready representation of a single
// event.
type Event struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Date        string   `json:"date"`      // YYYY-MM-DD, instance-local (model.Local)
	StartTime   string   `json:"startTime"` // HH:MM, instance-local (model.Local)
	EndTime     string   `json:"endTime"`   // HH:MM, instance-local (model.Local)
	Location    Location `json:"location"`
	Price       Price    `json:"price"`
	Organizer   string   `json:"organizer,omitempty"`
	URL         string   `json:"url,omitempty"`
	// SourceName is the public copy of Source.Name (set by writer.Write),
	// so the frontend can look up the source's label in data/sources.json.
	// "inbox" for hand-authored events.
	SourceName string `json:"source,omitempty"`
	// Sources lists every source that reported this event (winner first)
	// when the same event came from several sources and was merged (see
	// writer.MergeAcrossSources); a single entry otherwise.
	Sources []string `json:"sources,omitempty"`

	// Cross-cutting attributes, not categories: an event can be all, some or
	// none of these regardless of its Category. Deliberately real bool
	// fields (not pointers, no omitempty) so the frontend's attribute
	// filter row (web/js/app.js FILTER_OPTIONS: Kostenlos/Ticketpflichtig/
	// Anmeldung erforderlich/Familie & Kinder) always sees an explicit
	// true/false rather than an absent field defaulting ambiguously.
	//
	// TicketRequired and RegistrationRequired existed in the frontend and
	// in data/events.json's demo data from the start, but had no backing
	// field here - a real fetch run would have silently omitted them.
	// FamilyFriendly is new: it replaces "Familie & Kinder" as a category
	// value (see internal/categories and docs/backend.md -> "Kategorie-
	// Piktogramm-Schnittstelle") now that categories are audience-neutral
	// topic areas.
	TicketRequired       bool `json:"ticketRequired"`
	RegistrationRequired bool `json:"registrationRequired"`
	FamilyFriendly       bool `json:"familyFriendly"`

	// Cancelled marks an event the source reports as called off. Such
	// events are kept (and shown as "abgesagt" in the frontend) instead of
	// silently disappearing, so people who planned to go find out. Set by
	// the source (ical STATUS:CANCELLED, wcs hidden occurrences, a mapping
	// rule, the inbox) or by the title heuristic in withCommonFilters
	// ("Abgesagt: …").
	Cancelled bool `json:"cancelled"`

	Source   SourceRef `json:"-"`
	LastSeen time.Time `json:"-"`
}
