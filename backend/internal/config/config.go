// Package config loads <instance>/sources/*.json — one file per event
// source, declaring where to fetch it and how to map its fields onto
// model.Event. Adding, editing or removing a source touches exactly one
// file, which keeps git history/diffs/merges clean.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"openlitfass/backend/internal/mapping"
)

// SourceStatus records the outcome of the most recent fetch attempts for a
// source, written back into the source's own config file (see
// RecordAttempt) rather than a separate report file - so the error and the
// URL/mapping that produced it live in the same place.
type SourceStatus struct {
	// LastAttempt is when this source was last fetched at all, successful
	// or not.
	LastAttempt string `json:"lastAttempt,omitempty"`
	// LastSuccess is when this source last returned events without error.
	// Kept from the previous run if the most recent attempt failed, so you
	// can see both "when did it last work" and "when did it last try".
	LastSuccess string `json:"lastSuccess,omitempty"`
	// LastError is the plain-text error from the most recent attempt (e.g.
	// "fetch https://...: unexpected status 404 Not Found", "timestamp too
	// short", "panic: ..."). Empty when the most recent attempt succeeded.
	LastError string `json:"lastError,omitempty"`
}

// SourceConfig declares one event source. See
// <instance>/sources/*.example.json for fully worked RSS and JSON
// examples, and https://docs.openlitfass.org/?page=field-mapping for the mapping syntax.
type SourceConfig struct {
	// Name is a short, stable identifier (Event.Source.Name, stable id
	// derivation, log lines). Optional: defaults to the filename (without
	// .json) if omitted, so the file itself can be the single source of
	// truth for identity.
	Name string `json:"name,omitempty"`
	// Label is the human-readable source name shown in the frontend (event
	// detail "Quelle: ...", "Über"-Seite). Optional: defaults to Name.
	Label string `json:"label,omitempty"`
	// Homepage is the public website of the source (not the feed URL),
	// linked from the frontend's source list. Optional.
	Homepage string `json:"homepage,omitempty"`
	// Priority decides which source's version of an event is shown when
	// the same event arrives from several sources (higher wins, default 0).
	// Give a venue's own source a higher value than an aggregator calendar.
	Priority int `json:"priority,omitempty"`
	// Preset names a source preset shipped with the core (see Presets):
	// its config is the base, every field of this file overrides it.
	// Params fills the preset's {name} placeholders.
	Preset string            `json:"preset,omitempty"`
	Params map[string]string `json:"params,omitempty"`
	// Type selects the fetcher: "rss", "json", "ical" (see
	// internal/sources/ical.go) or "wcs" (WordPress "Weekly
	// Class Schedule" plugin by CurlyThemes — see internal/sources/wcs.go).
	Type string `json:"type,omitempty"`
	// URL is the feed/API endpoint to fetch. For "wcs" this is the site's
	// base URL (e.g. "https://example.org"), not the ajax endpoint itself.
	URL string `json:"url,omitempty"`
	// Disabled skips this source entirely (no fetch attempt, no status
	// update) without deleting or renaming the file - useful when a source
	// is temporarily broken or a venue is on hiatus.
	Disabled bool `json:"disabled,omitempty"`
	// ItemsPath (JSON sources only) is a dot-path to the array of items
	// within the decoded response, e.g. "data.items". Empty means the
	// top-level response is itself the array.
	ItemsPath string `json:"itemsPath,omitempty"`
	// NextPath (JSON sources only) is a dot-path to the URL of the next
	// page in the response (e.g. "next_rest_url" for WordPress "The Events
	// Calendar"). Pages are followed until it's empty, MaxPages is reached,
	// or a whole page lies beyond the DaysAhead window.
	NextPath string `json:"nextPath,omitempty"`
	// MaxPages caps pagination (default 10) so a misbehaving API can't
	// make a run loop forever.
	MaxPages int `json:"maxPages,omitempty"`
	// Steps run before the main request and collect variables from other
	// pages (see Step), e.g. the event ids a widget page lists before its
	// entries are loaded from a second endpoint. Works for every type that
	// fetches URL; see https://docs.openlitfass.org/?page=source-types#steps.
	Steps []Step `json:"steps,omitempty"`
	// ForEach repeats the main request for the values of a collected
	// variable (see ForEach). The events of all requests are combined.
	ForEach *ForEach `json:"forEach,omitempty"`
	// MaxRequests caps the main requests ForEach makes in one run
	// (default 20); the remaining values are skipped with a log line.
	MaxRequests int `json:"maxRequests,omitempty"`
	// TimeoutSeconds overrides the per-source time limit of 20 seconds,
	// for endpoints that render slowly (at most 180; the whole run has
	// five minutes).
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// ItemSelector ("html" sources only) is the CSS selector of one event
	// container on the page, e.g. "#jevents_body .schatten".
	ItemSelector string `json:"itemSelector,omitempty"`
	// SectionSelector ("html" sources only, optional) finds headings that
	// group the items without containing them, e.g. a date heading followed
	// by sibling entries ("<h3>Samstag, 21.11.2026</h3><div class=entry>…").
	// Each item gets the text of the last such heading before it (document
	// order) as the value "section", usable in mapping like an extract name.
	SectionSelector string `json:"sectionSelector,omitempty"`
	// Extract ("html" sources only) names the values pulled out of each
	// item; the mapping block then refers to these names as paths. See
	// ExtractRule and https://docs.openlitfass.org/?page=source-types#html.
	Extract map[string]ExtractRule `json:"extract,omitempty"`
	// Mapping declares how to translate one item into a model.Event. Only
	// used by "rss"/"json" sources. Pointer + omitempty so "wcs" sources
	// (which don't use it) don't get a large empty "mapping" block written
	// back into their file every time RecordAttempt runs.
	Mapping *mapping.SourceMapping `json:"mapping,omitempty"`

	// The two fields below apply uniformly to every source type ("rss",
	// "json", "wcs", ...) — enforced by withCommonFilters in
	// internal/sources/filter.go, wrapped around every source built in
	// All(). Already-past events are always dropped regardless of these
	// settings; that part isn't configurable.

	// DaysAhead, if set, drops events further out than today+N days
	// (instance timezone). Unset/0 = no window filter (rss/json keep whatever
	// the feed/API returns). "wcs" sources additionally use this to build
	// an efficient request window (defaulting to 30 there if unset) rather
	// than fetching everything and filtering client-side.
	DaysAhead int `json:"daysAhead,omitempty"`
	// Categories, if set, keeps only events whose resolved Category (after
	// any source-specific mapping/CategoryMap) is in this list. Unset/empty
	// = no category filter.
	Categories []string `json:"categories,omitempty"`
	// Include keeps only events where at least one rule matches, Exclude
	// drops events where any rule matches - e.g. to take from a source
	// only what happens in the neighbourhood:
	// "include": [{"field": "location", "regex": "(?i)weißensee|1308[68]"}].
	// Fields: title, location (name and address), organizer, category,
	// description. Applied after category mapping.
	Include []MatchRule `json:"include,omitempty"`
	Exclude []MatchRule `json:"exclude,omitempty"`

	// The fields below apply to "wcs" and "ical" sources (WCSTypeIDs: wcs
	// only; CategoryMap: every type).

	// WCSTypeIDs filters by the plugin's "wcs_type" taxonomy term IDs at
	// request time (not a post-filter like Categories above — this changes
	// what the remote endpoint returns in the first place).
	// Important: WCS sites are commonly set up from a plugin demo/template
	// and can still carry unrelated demo classes (dance/gym/school sample
	// data) with no bearing on the real venue — always set this explicitly
	// rather than leaving it empty. Find the IDs by opening the site's own
	// calendar in a browser, checking the network request its widget makes
	// to admin-ajax.php?action=wcs_get_events_json, and reading off the
	// content[wcs_type][] values used there.
	WCSTypeIDs []int `json:"wcsTypeIds,omitempty"`
	// Organizer is used for every event (wcs: the plugin's API doesn't
	// expose one; ical: when an event has no ORGANIZER;CN).
	Organizer string `json:"organizer,omitempty"`
	// LocationDefault is "Venue, Street 1, 12345 City". wcs: used for every
	// event - the venue becomes location.name, the full string the address,
	// with a "wcs_room" term inserted after the venue. ical: used as name
	// when an event has no LOCATION.
	LocationDefault string `json:"locationDefault,omitempty"`
	// CategoryMap translates the source's own category names to the
	// instance's vocabulary (see <instance>/categories.json), e.g.
	// {"Kino": "Kultur", "Kurs": "Kultur"}, so events get an icon instead
	// of showing up as an unknown category. Works for every source type
	// (applied in withCommonFilters, keys matched case-insensitively,
	// before the Categories allowlist). For "wcs" sources the keys can also
	// be wcs_type slugs (matched against the slug first, see wcs.go).
	// Unmapped values are kept as-is.
	CategoryMap map[string]string `json:"categoryMap,omitempty"`
	// FamilyFriendly is applied to every event this source produces (see
	// model.Event.FamilyFriendly) - a per-event signal isn't available from
	// the wcs API, but for a source that IS a family/parenting venue this is
	// a verified fact about the source itself, not a guess. Leave unset
	// (false) for sources where family-friendliness varies event to event.
	FamilyFriendly bool `json:"familyFriendly,omitempty"`
	// Place links every event of this source to one place of
	// <instance>/places.json: its id or one of its refs. For sources with a
	// single venue; events that carry their own reference (mapping
	// "placeRef") keep it. Applied in withCommonFilters.
	Place string `json:"place,omitempty"`
	// Images keeps the event images this source provides (mapping "image",
	// iCal IMAGE/ATTACH). Off by default because the licence of such images
	// varies; turn it on only where the source allows the use. The image is
	// never copied, only referenced by its URL. Applied in withCommonFilters.
	Images bool `json:"images,omitempty"`

	// Status is maintained by the pipeline itself (see RecordAttempt) - not
	// something you write by hand. Absent until the first fetch attempt.
	Status *SourceStatus `json:"status,omitempty"`

	// Path is the file this config was loaded from. Not part of the JSON -
	// set by Load, used by internal/sources to call RecordAttempt after a
	// fetch so status lands back in the right file.
	Path string `json:"-"`
}

// Load reads every *.json file directly inside dir — one source per file
// (see <instance>/sources/) — and returns their parsed SourceConfigs,
// ordered by filename for deterministic runs.
//
// Files ending in ".example.json" are skipped: they're copy-paste templates,
// not live sources. A missing directory is treated as zero configured
// sources rather than an error, so the repo works before any source file
// has been added.
//
// A single malformed source file is logged and skipped rather than failing
// the whole load — one typo in one file shouldn't take every other,
// perfectly valid source down with it. Load only returns an error if the
// directory itself can't be listed.
func Load(dir string) ([]SourceConfig, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".example.json") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	configs := make([]SourceConfig, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("skipping %s: %v", path, err)
			continue
		}
		c, err := parseSource(data)
		if err != nil {
			log.Printf("skipping %s: %v", path, err)
			continue
		}
		if c.Name == "" {
			c.Name = strings.TrimSuffix(name, ".json")
		}
		c.Path = path
		configs = append(configs, c)
	}
	return configs, nil
}

// RecordAttempt writes the outcome of a fetch attempt back into path's own
// JSON, updating only the "status" field. fetchErr is nil on success.
//
// It re-reads path fresh from disk rather than reusing an in-memory
// SourceConfig, so it never re-derives/writes a "name" that the file didn't
// already have (Load's filename-fallback is a runtime convenience, not
// something that should leak into the file on disk).
//
// This re-serializes the whole file via SourceConfig, so field order
// follows the Go struct's declared order (stable across runs, but not
// necessarily how a human originally formatted it), and any JSON keys not
// modeled by SourceConfig are dropped. Given the schema is the single
// source of truth for what a source file may contain, that's an accepted
// trade-off for keeping this simple.
func RecordAttempt(path string, fetchErr error) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var c SourceConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	status := SourceStatus{LastAttempt: now}
	if c.Status != nil {
		status.LastSuccess = c.Status.LastSuccess
	}
	if fetchErr != nil {
		status.LastError = fetchErr.Error()
	} else {
		status.LastSuccess = now
	}
	c.Status = &status
	c.Path = "" // json:"-" anyway, but be explicit that it never gets written

	// No HTML escaping: source files are hand-written and may contain CSS
	// selectors ("div > p") and regexes ("<b>") that json.Marshal would
	// turn into > / <.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// MatchRule is one entry of SourceConfig.Include / Exclude: Regex is
// matched against the event field named by Field.
type MatchRule struct {
	Field string `json:"field"`
	Regex string `json:"regex"`
}
