// Package writer turns collected events into the deterministic JSON files
// the frontend fetches at runtime: data/events.json (the events
// themselves) and data/categories.json (the category->pictogram
// translation, see internal/categories).
package writer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"openlitfass/backend/internal/categories"
	"openlitfass/backend/internal/model"
)

// Output is the top-level shape of data/events.json. "events" is the only
// key the frontend reads; generatedAt is additive metadata it's free to
// ignore.
type Output struct {
	GeneratedAt time.Time     `json:"generatedAt"`
	Events      []model.Event `json:"events"`
}

// CategoriesOutput is the top-level shape of data/categories.json - see
// internal/categories for what goes into "categories" and why this file
// exists as a one-way, backend-writes/frontend-reads interface.
type CategoriesOutput struct {
	GeneratedAt time.Time          `json:"generatedAt"`
	Categories  []categories.Entry `json:"categories"`
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = nonSlugChars.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// MakeID derives a stable, readable id from a source name + its internal id,
// so the same real-world event gets the same Event.ID across runs, e.g.
// "kulturzentrum-tiefgrund-3f9a1b2c4d".
func MakeID(sourceName, sourceID string) string {
	sum := sha256.Sum256([]byte(sourceName + "|" + sourceID))
	return slugify(sourceName) + "-" + hex.EncodeToString(sum[:])[:10]
}

// validate drops events too broken to show sensibly in the frontend
// (missing title, or a date that isn't a plausible YYYY-MM-DD) instead of
// silently writing them into data/events.json. A single source producing
// garbage for one item (a mapping typo, an unexpected null in a feed, ...)
// shouldn't corrupt the calendar - it should show up as a loud log line and
// otherwise be skipped, the same way a failing source is skipped rather
// than treated as fatal (see cmd/fetch/main.go).
func validate(events []model.Event) []model.Event {
	kept := make([]model.Event, 0, len(events))
	dropped := 0
	for _, e := range events {
		if strings.TrimSpace(e.Title) == "" || !looksLikeDate(e.Date) {
			dropped++
			continue
		}
		kept = append(kept, e)
	}
	if dropped > 0 {
		log.Printf("dropping %d event(s) with missing title or invalid date", dropped)
	}
	return kept
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func looksLikeDate(s string) bool {
	return dateRe.MatchString(s)
}

// Dedupe merges events sharing the same (source name, source id), keeping
// whichever copy has the most recent LastSeen. Order of first appearance is
// preserved for events that don't collide.
func Dedupe(events []model.Event) []model.Event {
	latest := make(map[string]model.Event, len(events))
	order := make([]string, 0, len(events))
	for _, e := range events {
		key := e.Source.Name + "|" + e.Source.SourceID
		existing, ok := latest[key]
		if !ok {
			order = append(order, key)
			latest[key] = e
			continue
		}
		if e.LastSeen.After(existing.LastSeen) {
			latest[key] = e
		}
	}
	result := make([]model.Event, 0, len(order))
	for _, key := range order {
		result = append(result, latest[key])
	}
	return result
}

// SortEvents orders events by date/startTime (then title) so re-runs with
// unchanged data produce a byte-identical file and clean git diffs.
func SortEvents(events []model.Event) {
	sort.Slice(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.StartTime != b.StartTime {
			return a.StartTime < b.StartTime
		}
		return a.Title < b.Title
	})
}

// Write assigns stable ids, validates, dedupes, sorts and writes events as
// pretty JSON to path. Parent directories are created as needed. It returns
// the final, validated/deduped/sorted event list - the same one written to
// path - so callers can feed the exact same set into WriteCategories
// instead of a stale pre-validation copy (see cmd/fetch/main.go).
//
// It refuses to write an empty event list: a source outage, a not-yet-
// implemented All() (see sources.All), or every event failing validation
// should never wipe out the last known good data/events.json. Callers
// should check for this case (see cmd/fetch/main.go) and skip the write
// entirely rather than treating it as an error.
//
// baseURL (instance.json "baseUrl") is the fallback link for events whose
// source provides none: the instance site itself, anchored to the event id.
// Empty baseURL leaves such events without a URL.
func Write(path string, events []model.Event, baseURL string) ([]model.Event, error) {
	for i := range events {
		if events[i].ID == "" {
			events[i].ID = MakeID(events[i].Source.Name, events[i].Source.SourceID)
		}
		if events[i].SourceName == "" {
			events[i].SourceName = events[i].Source.Name
		}
		if events[i].URL == "" && baseURL != "" {
			events[i].URL = strings.TrimRight(baseURL, "/") + "/#" + events[i].ID
		}
	}

	events = validate(events)
	if len(events) == 0 {
		return nil, fmt.Errorf("refusing to write %s: zero valid events (would overwrite existing data)", path)
	}

	events = Dedupe(events)
	SortEvents(events)

	out := Output{
		GeneratedAt: time.Now().UTC(),
		Events:      events,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal events: %w", err)
	}

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create output dir %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	return events, nil
}

// WriteCategories writes the category->pictogram translation for events to
// path (see internal/categories.Config.Build). This is the backend's half
// of a one-way interface: the frontend only ever reads this file to render
// the category filter menu and event-card icons, never the other way
// around - see docs/backend.md -> "Kategorie-Piktogramm-Schnittstelle".
//
// cfg is the loaded <instance>/categories.json (see categories.Load,
// called once in cmd/fetch/main.go) - the hand-edited registry this
// generated file is built from.
//
// Pass Write's returned event list here, not the raw pre-validation one, so
// data/categories.json reflects exactly what's in data/events.json.
func WriteCategories(path string, cfg categories.Config, events []model.Event) error {
	out := CategoriesOutput{
		GeneratedAt: time.Now().UTC(),
		Categories:  cfg.Build(events),
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal categories: %w", err)
	}

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output dir %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// SourceInfo is one row of data/sources.json: the public, human-readable
// view of a configured source for the frontend's "Über" page and the
// "Quelle: ..." line in the event detail. Deliberately no feed URLs or
// error messages - those stay in the source files (see config.SourceStatus).
type SourceInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Homepage    string `json:"homepage,omitempty"`
	LastSuccess string `json:"lastSuccess,omitempty"`
}

// SourcesOutput is the top-level shape of data/sources.json.
type SourcesOutput struct {
	GeneratedAt time.Time    `json:"generatedAt"`
	Sources     []SourceInfo `json:"sources"`
}

// WriteSources writes data/sources.json.
func WriteSources(path string, sources []SourceInfo) error {
	out := SourcesOutput{GeneratedAt: time.Now().UTC(), Sources: sources}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sources: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
