// Package categories is the single canonical translation between an
// Event's Category string and the pictogram used to represent it in the
// frontend.
//
// This used to be hardcoded twice, independently: once as CATEGORY_MAP
// values in source configs (see <instance>/sources/), and once as
// CATEGORY_ICON_PATHS directly in web/js/app.js, with nothing keeping the two
// in sync. This package became the single owner of that translation, and
// its data now lives in <instance>/categories.json rather than in Go
// source (see Load) - a plain JSON file is a much smaller, more obvious
// diff to review and edit by hand than a Go map literal, and it's the same
// format the file it ultimately produces (data/categories.json) already
// uses, so there's nothing new to learn.
//
// internal/writer writes the built result to data/categories.json on every
// run, and web/js/app.js only ever reads that file to render the category
// filter menu and event-card icons - it must not hardcode or invent icons
// of its own again. See docs/backend.md -> "Kategorie-Piktogramm-
// Schnittstelle" for the full contract, including how this differs from
// <instance>/categories.json (the hand-edited input) below.
package categories

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"

	"openlitfass/backend/internal/model"
)

// Entry is one row of data/categories.json, and - same shape, reused
// rather than duplicated - also one row of the "categories" array in
// <instance>/categories.json (see rawFile).
type Entry struct {
	Name string   `json:"name"`
	Icon []string `json:"icon"`
}

// Config is the loaded, ready-to-use form of <instance>/categories.json:
// Registry for O(1) name->icon lookups, Order for the fixed display
// sequence (the same order the JSON file lists categories in - the same
// role web/js/app.js's old Object.keys() insertion order used to play), and
// Fallback for the name whose icon stands in for any Category value
// Registry doesn't recognize (see Build).
type Config struct {
	Registry map[string][]string
	Order    []string
	Fallback string
}

// rawFile is the on-disk shape of <instance>/categories.json - kept
// separate from Config so Load can fully validate the file (duplicate
// names, missing icons, an unresolvable fallback, ...) before handing
// callers something that's actually convenient to use, instead of
// re-deriving a map from a slice on every Build call.
type rawFile struct {
	Fallback   string  `json:"fallback"`
	Categories []Entry `json:"categories"`
}

// Load reads and validates path (see <instance>/categories.json),
// building the Registry/Order/Fallback trio Build needs.
//
// This is load-bearing, not best-effort, deliberately: a missing or broken
// category registry would mean every event silently loses its icon in the
// frontend, which is a much worse failure mode than cmd/fetch refusing to
// run until the file is fixed - the same reasoning writer.WriteCategories
// already applies to data/categories.json itself (see cmd/fetch/main.go).
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	var raw rawFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}

	if raw.Fallback == "" {
		return Config{}, fmt.Errorf(`%s: "fallback" is required`, path)
	}
	if len(raw.Categories) == 0 {
		return Config{}, fmt.Errorf(`%s: "categories" is empty`, path)
	}

	registry := make(map[string][]string, len(raw.Categories))
	order := make([]string, 0, len(raw.Categories))
	for _, c := range raw.Categories {
		if c.Name == "" {
			return Config{}, fmt.Errorf(`%s: a category entry is missing "name"`, path)
		}
		if len(c.Icon) == 0 {
			return Config{}, fmt.Errorf(`%s: category %q has no "icon" path data`, path, c.Name)
		}
		if _, dup := registry[c.Name]; dup {
			return Config{}, fmt.Errorf("%s: category %q is listed twice", path, c.Name)
		}
		registry[c.Name] = c.Icon
		order = append(order, c.Name)
	}

	if _, ok := registry[raw.Fallback]; !ok {
		return Config{}, fmt.Errorf("%s: fallback %q is not one of the listed categories", path, raw.Fallback)
	}

	return Config{Registry: registry, Order: order, Fallback: raw.Fallback}, nil
}

// Build returns the full category menu for a set of events: every category
// in c.Order, always, in that order - deliberately independent of which
// categories currently have events, so the menu bar stays visually stable
// run to run - plus any Category value actually present in events that
// c.Registry doesn't know about yet, appended alphabetically with
// c.Fallback's icon.
//
// That second part is what makes the interface handle categories "growing
// dynamically": a new source (or a wcs categoryMap gap) can introduce a
// category nobody has taught the registry about, and instead of the run
// failing or the value silently vanishing, it still shows up - with a
// generic icon and a loud log line pointing at exactly where to add a
// dedicated one.
func (c Config) Build(events []model.Event) []Entry {
	known := make(map[string]bool, len(c.Order))
	entries := make([]Entry, 0, len(c.Order))
	for _, name := range c.Order {
		entries = append(entries, Entry{Name: name, Icon: c.Registry[name]})
		known[name] = true
	}

	seen := make(map[string]bool)
	var newOnes []string
	for _, e := range events {
		if e.Category == "" || known[e.Category] || seen[e.Category] {
			continue
		}
		seen[e.Category] = true
		newOnes = append(newOnes, e.Category)
	}
	sort.Strings(newOnes)

	for _, name := range newOnes {
		log.Printf("category %q has no dedicated icon yet, falling back to %q - add it to <instance>/categories.json", name, c.Fallback)
		entries = append(entries, Entry{Name: name, Icon: c.Registry[c.Fallback]})
	}

	return entries
}
